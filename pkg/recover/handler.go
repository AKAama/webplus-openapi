package recover

import (
	"fmt"
	"runtime"
	"sync"
	"time"
	"webplus-openapi/pkg/models"

	"github.com/pkg/errors"
	"github.com/timshannon/badgerhold/v4"
	"go.uber.org/zap"
)

// RecoverHistoryData 恢复历史数据的主入口
func (s *Service) RecoverHistoryData(params Params) error {
	// 参数验证
	if err := s.validateParams(params); err != nil {
		return fmt.Errorf("参数验证失败: %w", err)
	}

	zap.S().Info("开始数据恢复任务")

	// 创建文章服务
	articleRepo := NewArticleRepository(s.db)
	articleService := NewArticleService(articleRepo, s.badgerStore)

	// 获取所有需要恢复的文章ID
	articleRefs, err := articleRepo.GetAllArticleRefs(params)
	if err != nil {
		return fmt.Errorf("获取文章列表失败: %w", err)
	}

	zap.S().Infof("共找到 %d 篇文章需要恢复", len(articleRefs))

	if len(articleRefs) == 0 {
		zap.S().Info("没有需要恢复的文章")
		return nil
	}
	startTime := time.Now()
	// 执行并发批量恢复
	result, err := articleService.ProcessArticlesConcurrently(articleRefs, params)
	if err != nil {
		return fmt.Errorf("并发批量处理失败: %w", err)
	}
	duration := time.Since(startTime)

	// 输出最终统计
	zap.S().Infof("数据恢复任务完成 - 总文章: %d, 实际处理: %d, 跳过: %d, 错误: %d ,耗时：%v",
		len(articleRefs), result.ProcessedCount, result.SkippedCount, result.ErrorCount, duration)

	return nil
}

// validateParams 验证恢复参数
func (s *Service) validateParams(params Params) error {
	if params.BatchSize <= 0 {
		return errors.New("批次大小必须大于0")
	}
	if params.BatchSize > 1000 {
		zap.S().Warnf("批次大小 %d 较大，建议使用更小的批次以提高稳定性", params.BatchSize)
	}

	// 设置默认并发数
	if params.Concurrency <= 0 {
		params.Concurrency = runtime.NumCPU()
		zap.S().Infof("使用默认并发数: %d (CPU核心数)", params.Concurrency)
	}
	if params.Concurrency > 20 {
		zap.S().Warnf("并发数 %d 较大，建议使用更小的并发数以提高稳定性", params.Concurrency)
	}

	// 设置默认Worker池大小
	if params.WorkerPoolSize <= 0 {
		params.WorkerPoolSize = params.Concurrency * 2
		zap.S().Infof("使用默认Worker池大小: %d", params.WorkerPoolSize)
	}

	return nil
}

// GetAllArticleRefs 获取所有需要恢复的文章引用
func (r *ArticleRepository) GetAllArticleRefs(params Params) ([]ArticleRef, error) {
	query := r.db.Table("T_ARTICLE ta").
		Select("ta.id, tsa.siteId as site_id").
		Joins("JOIN T_SITEARTICLE tsa ON ta.id = tsa.publishArticleId").
		Joins("Join T_PUBLISHSITE tps ON tsa.siteId = tps.siteId").
		Where("tsa.published = ? AND tsa.selfCreate = ? AND ta.deleted = ? AND ta.archived = ?", 1, 1, 0, 0)

	// 添加站点过滤条件（空字符串表示所有站点）
	if params.SiteID != "" {
		query = query.Where("tsa.siteId = ?", params.SiteID)
	}

	var articles []ArticleRef
	if err := query.Find(&articles).Error; err != nil {
		return nil, fmt.Errorf("查询文章列表失败: %w", err)
	}

	return articles, nil
}

// ProcessArticlesInBatches 批量处理文章
func (as *ArticleService) ProcessArticlesInBatches(articles []ArticleRef, batchSize int) (*BatchResult, error) {
	if len(articles) == 0 {
		return &BatchResult{}, nil
	}

	totalResult := &BatchResult{}
	totalBatches := (len(articles) + batchSize - 1) / batchSize

	zap.S().Infof("开始批量处理，共 %d 篇文章，分 %d 个批次", len(articles), totalBatches)

	// 分批处理
	for i := 0; i < len(articles); i += batchSize {
		end := i + batchSize
		if end > len(articles) {
			end = len(articles)
		}

		batch := articles[i:end]
		batchNum := (i / batchSize) + 1
		zap.S().Debugf("处理批次 [%d/%d] [%d-%d]", batchNum, totalBatches, i+1, end)

		// 处理当前批次，增加重试机制
		batchResult, err := as.processBatchWithRetry(batch, 3)
		if err != nil {
			zap.S().Errorf("批次 [%d/%d] 处理失败: %v", batchNum, totalBatches, err)
			// 继续处理下一批次，而不是完全失败
			totalResult.ErrorCount += len(batch)
			continue
		}

		// 累计统计
		totalResult.ProcessedCount += batchResult.ProcessedCount
		totalResult.SkippedCount += batchResult.SkippedCount
		totalResult.ErrorCount += batchResult.ErrorCount

		// 批次完成日志
		zap.S().Infof("批次 [%d/%d] 完成 - 处理: %d, 跳过: %d, 错误: %d",
			batchNum, totalBatches, batchResult.ProcessedCount, batchResult.SkippedCount, batchResult.ErrorCount)

		// 进度报告
		progress := float64(batchNum) / float64(totalBatches) * 100
		zap.S().Infof("整体进度: %.1f%% (%d/%d 批次)", progress, batchNum, totalBatches)
	}

	return totalResult, nil
}

// ProcessArticlesConcurrently 并发处理文章
func (as *ArticleService) ProcessArticlesConcurrently(articles []ArticleRef, params Params) (*BatchResult, error) {
	if len(articles) == 0 {
		zap.S().Warn("没有文章需要处理")
		return &BatchResult{}, nil
	}

	zap.S().Infof("开始处理 %d 篇文章，批次大小: %d", len(articles), params.BatchSize)

	// 创建批次
	batches := as.createBatches(articles, params.BatchSize)
	totalBatches := len(batches)

	zap.S().Infof("分 %d 个批次处理", totalBatches)

	result := &BatchResult{}

	// 并发处理每个批次，限制批次数并发度
	concurrency := params.Concurrency
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}
	if concurrency > 32 {
		concurrency = 32
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex

	for i, batch := range batches {
		batchNum := i + 1
		batch := batch

		wg.Add(1)
		sem <- struct{}{}
		go func(num int) {
			defer wg.Done()
			defer func() { <-sem }()

			zap.S().Infof("处理批次 [%d/%d]，包含 %d 篇文章", num, totalBatches, len(batch))

			batchResult, err := as.processBatchWithRetry(batch, 3)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				zap.S().Errorf("批次 [%d/%d] 处理失败: %v", num, totalBatches, err)
				// 将整个批次标记为错误
				result.ErrorCount += len(batch)
				return
			}

			// 累计统计
			result.ProcessedCount += batchResult.ProcessedCount
			result.SkippedCount += batchResult.SkippedCount
			result.ErrorCount += batchResult.ErrorCount

			// 批次完成日志
			zap.S().Infof("批次 [%d/%d] 完成 - 处理: %d, 跳过: %d, 错误: %d",
				num, totalBatches, batchResult.ProcessedCount, batchResult.SkippedCount, batchResult.ErrorCount)

			// 进度报告
			progress := float64(num) / float64(totalBatches) * 100
			zap.S().Infof("整体进度: %.1f%% (%d/%d 批次)", progress, num, totalBatches)
		}(batchNum)
	}

	wg.Wait()

	zap.S().Infof("处理完成 - 总文章: %d, 实际处理: %d, 跳过: %d, 错误: %d",
		len(articles), result.ProcessedCount, result.SkippedCount, result.ErrorCount)

	return result, nil
}

// createBatches 创建批次
func (as *ArticleService) createBatches(articles []ArticleRef, batchSize int) [][]ArticleRef {
	var batches [][]ArticleRef

	for i := 0; i < len(articles); i += batchSize {
		end := i + batchSize
		if end > len(articles) {
			end = len(articles)
		}
		batches = append(batches, articles[i:end])
	}

	return batches
}

// processBatchWithRetry 带重试机制的批次处理
func (as *ArticleService) processBatchWithRetry(articles []ArticleRef, maxRetries int) (*BatchResult, error) {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		result, err := as.processBatch(articles)
		if err == nil {
			return result, nil
		}

		lastErr = err
		if attempt < maxRetries {
			zap.S().Warnf("批次处理失败，第 %d 次重试 (共 %d 次): %v", attempt, maxRetries, err)
		}
	}

	return nil, fmt.Errorf("批次处理失败，已重试 %d 次: %w", maxRetries, lastErr)
}

// processBatch 处理单个批次
func (as *ArticleService) processBatch(articles []ArticleRef) (*BatchResult, error) {
	if len(articles) == 0 {
		return &BatchResult{}, nil
	}

	result := &BatchResult{}

	// 顺序处理每篇文章
	for _, article := range articles {
		processResult := as.processArticle(article)
		switch processResult.Status {
		case "processed":
			result.ProcessedCount++
		case "skipped":
			result.SkippedCount++
		default: // 错误信息
			result.ErrorCount++
			zap.S().Errorf("处理文章 %s 失败: %s", article.ID, processResult.Status)
		}
	}

	return result, nil
}

// processArticle 处理单篇文章
func (as *ArticleService) processArticle(articleRef ArticleRef) ProcessResult {
	// 检查文章是否已存在
	var existingArticle models.ArticleInfo
	err := as.badgerStore.Get(articleRef.ID, &existingArticle)
	if err == nil {
		zap.S().Debugf("文章 %s 已存在于BadgerDB中，跳过处理", articleRef.ID)
		return ProcessResult{Status: "skipped"}
	}
	if !errors.Is(err, badgerhold.ErrNotFound) {
		return ProcessResult{Status: fmt.Sprintf("检查文章存在性失败: %v", err)}
	}

	// 创建Article对象（仅携带ID；站点与栏目从数据库计算）
	article := &models.ArticleInfo{
		ArticleId: articleRef.ID,
	}

	// 查询文章详情
	articleInfo, err := as.repo.GetArticleById(article)
	if err != nil {
		return ProcessResult{Status: fmt.Sprintf("查询文章详情失败: %v", err)}
	}

	// 修复文章访问地址
	articleInfo, err = as.repo.RestoreArticleInfos(articleInfo)
	if err != nil {
		return ProcessResult{Status: fmt.Sprintf("修复文章访问地址失败: %v", err)}
	}

	// 存储文章
	if err := as.badgerStore.Store(articleInfo.ArticleId, articleInfo); err != nil {
		return ProcessResult{Status: fmt.Sprintf("存储文章到BadgerDB失败: %v", err)}
	}

	zap.S().Debugf("成功恢复文章 %s 到BadgerDB", articleRef.ID)
	return ProcessResult{Status: "processed"}
}
