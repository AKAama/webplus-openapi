package recover

import (
	"context"
	"fmt"
	"runtime"
	"sync"
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

	// 执行并发批量恢复
	result, err := articleService.ProcessArticlesConcurrently(articleRefs, params)
	if err != nil {
		return fmt.Errorf("并发批量处理失败: %w", err)
	}

	// 输出最终统计
	zap.S().Infof("数据恢复任务完成 - 总文章: %d, 实际处理: %d, 跳过: %d, 错误: %d",
		len(articleRefs), result.ProcessedCount, result.SkippedCount, result.ErrorCount)

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

	// 验证站点ID格式（如果提供）
	if params.SiteID != "" && len(params.SiteID) < 3 {
		return errors.New("站点ID格式不正确")
	}

	// 验证栏目ID格式（如果提供）
	if params.ColumnID != "" && len(params.ColumnID) < 3 {
		return errors.New("栏目ID格式不正确")
	}

	return nil
}

// GetAllArticleRefs 获取所有需要恢复的文章引用
func (r *ArticleRepository) GetAllArticleRefs(params Params) ([]ArticleRef, error) {
	query := r.db.Table("T_ARTICLE ta").
		Select("ta.id, tsa.siteId as site_id, tc.id as column_id").
		Joins("JOIN T_SITEARTICLE tsa ON ta.id = tsa.publishArticleId").
		Joins("JOIN T_COLUMN tc ON tc.SyncFolderId = ta.folderId").
		Where("tsa.selfCreate = ?", 1)

	// 添加站点过滤条件（空字符串表示所有站点）
	if params.SiteID != "" {
		query = query.Where("tsa.siteId = ?", params.SiteID)
	}

	// 添加栏目过滤条件（空字符串表示所有栏目）
	if params.ColumnID != "" {
		query = query.Where("tc.id = ?", params.ColumnID)
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
		return &BatchResult{}, nil
	}

	// 创建批次
	batches := as.createBatches(articles, params.BatchSize)
	totalBatches := len(batches)

	zap.S().Infof("开始并发处理，共 %d 篇文章，分 %d 个批次，并发数: %d",
		len(articles), totalBatches, params.Concurrency)

	// 创建进度跟踪器
	progressTracker := NewProgressTracker(totalBatches)

	// 创建上下文和取消函数
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建批次通道
	batchChan := make(chan *ConcurrentBatch, params.Concurrency*2)
	resultChan := make(chan *ConcurrentBatch, params.Concurrency*2)

	// 启动Worker池
	var wg sync.WaitGroup
	for i := 0; i < params.Concurrency; i++ {
		wg.Add(1)
		go as.worker(ctx, i+1, batchChan, resultChan, &wg)
	}

	// 启动结果收集器
	go as.resultCollector(ctx, resultChan, progressTracker, totalBatches)

	// 发送批次到Worker池
	go func() {
		defer close(batchChan)
		for i, batch := range batches {
			select {
			case batchChan <- &ConcurrentBatch{
				ID:       i + 1,
				Articles: batch,
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// 等待所有Worker完成
	wg.Wait()
	close(resultChan)

	// 获取最终结果
	completed, total, processed, skipped, errors := progressTracker.GetProgress()
	zap.S().Infof("并发处理完成 - 批次: %d/%d, 处理: %d, 跳过: %d, 错误: %d",
		completed, total, processed, skipped, errors)

	return &BatchResult{
		ProcessedCount: processed,
		SkippedCount:   skipped,
		ErrorCount:     errors,
	}, nil
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

// worker Worker协程
func (as *ArticleService) worker(ctx context.Context, workerID int, batchChan <-chan *ConcurrentBatch, resultChan chan<- *ConcurrentBatch, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case batch, ok := <-batchChan:
			if !ok {
				zap.S().Debugf("Worker %d 退出", workerID)
				return
			}

			zap.S().Debugf("Worker %d 开始处理批次 %d", workerID, batch.ID)

			// 处理批次
			result, err := as.processBatchWithRetry(batch.Articles, 3)
			batch.Result = result
			batch.Error = err

			// 发送结果
			select {
			case resultChan <- batch:
				zap.S().Debugf("Worker %d 完成批次 %d", workerID, batch.ID)
			case <-ctx.Done():
				return
			}

		case <-ctx.Done():
			zap.S().Debugf("Worker %d 被取消", workerID)
			return
		}
	}
}

// resultCollector 结果收集器
func (as *ArticleService) resultCollector(ctx context.Context, resultChan <-chan *ConcurrentBatch, progressTracker *ProgressTracker, totalBatches int) {
	completedBatches := 0

	for {
		select {
		case batch, ok := <-resultChan:
			if !ok {
				return
			}

			completedBatches++

			if batch.Error != nil {
				zap.S().Errorf("批次 %d 处理失败: %v", batch.ID, batch.Error)
				// 将整个批次标记为错误
				progressTracker.UpdateProgress(&BatchResult{
					ProcessedCount: 0,
					SkippedCount:   0,
					ErrorCount:     len(batch.Articles),
				})
			} else {
				zap.S().Infof("批次 %d 完成 - 处理: %d, 跳过: %d, 错误: %d",
					batch.ID, batch.Result.ProcessedCount, batch.Result.SkippedCount, batch.Result.ErrorCount)
				progressTracker.UpdateProgress(batch.Result)
			}

			// 报告进度
			completed, total, processed, skipped, errors := progressTracker.GetProgress()
			progress := float64(completed) / float64(total) * 100
			zap.S().Infof("进度: %.1f%% (%d/%d 批次) - 处理: %d, 跳过: %d, 错误: %d",
				progress, completed, total, processed, skipped, errors)

		case <-ctx.Done():
			return
		}
	}
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

	// 创建Article对象
	article := &models.ArticleInfo{
		ArticleId: articleRef.ID,
		SiteId:    articleRef.SiteID,
		ColumnId:  articleRef.ColumnID,
	}

	// 查询文章详情
	articleInfo, err := as.repo.GetArticleById(article)
	if err != nil {
		return ProcessResult{Status: fmt.Sprintf("查询文章详情失败: %v", err)}
	}

	// 修复文章访问地址
	articleInfo, err = as.repo.RestoreArticleInfos(articleInfo, article)
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
