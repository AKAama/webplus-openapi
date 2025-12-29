package sync

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
	"webplus-openapi/pkg/models"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Syncable 可同步的实体接口
type Syncable interface {
	GetID() int
	TableName() string
}

// TableSyncService 通用表同步服务
type TableSyncService struct {
	sourceDB    *gorm.DB
	targetDB    *gorm.DB
	tableName   string
	entityType  reflect.Type
	hasChanged  func(old, new interface{}) bool
	mu          sync.Mutex
	running     bool
	serviceName string // 用于日志显示
}

// NewTableSyncService 创建通用表同步服务
func NewTableSyncService(sourceDB, targetDB *gorm.DB, tableName string, entityType reflect.Type, hasChanged func(old, new interface{}) bool, serviceName string) *TableSyncService {
	return &TableSyncService{
		sourceDB:    sourceDB,
		targetDB:    targetDB,
		tableName:   tableName,
		entityType:  entityType,
		hasChanged:  hasChanged,
		serviceName: serviceName,
	}
}

// Sync 执行同步操作
func (s *TableSyncService) Sync() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("同步任务正在运行中，请稍后再试")
	}
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	if s.sourceDB == nil {
		return fmt.Errorf("SourceDB 未初始化")
	}
	if s.targetDB == nil {
		return fmt.Errorf("targetDB 未初始化")
	}

	zap.S().Infof("开始同步 %s 表...", s.serviceName)
	startTime := time.Now()

	// 1. 从 SourceDB 读取所有数据
	// 创建 []*Entity 类型的切片
	sliceType := reflect.SliceOf(reflect.PtrTo(s.entityType))
	sourcePtr := reflect.New(sliceType)
	sourceSlice := sourcePtr.Elem()

	if err := s.sourceDB.Table(s.tableName).Find(sourcePtr.Interface()).Error; err != nil {
		return fmt.Errorf("从 SourceDB 读取数据失败: %w", err)
	}

	sourceLen := sourceSlice.Len()
	zap.S().Infof("从 SourceDB 读取到 %d 条数据", sourceLen)

	// 2. 从 targetDB 读取现有数据
	targetPtr := reflect.New(sliceType)
	targetSlice := targetPtr.Elem()

	if err := s.targetDB.Table(s.tableName).Find(targetPtr.Interface()).Error; err != nil {
		zap.S().Warnf("从 targetDB 读取数据失败（可能表不存在）: %v", err)
		targetSlice.Set(reflect.MakeSlice(sliceType, 0, 0))
	}

	targetLen := targetSlice.Len()

	// 3. 构建现有数据的映射（以 Id 为 key）
	targetMap := make(map[int]interface{})
	for i := 0; i < targetLen; i++ {
		item := targetSlice.Index(i).Interface()
		id := s.getIdFromEntity(item)
		targetMap[id] = item
	}

	// 4. 统计变更
	var added, updated, deleted int

	// 5. 开始事务
	tx := s.targetDB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("开启事务失败: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			zap.S().Errorf("同步过程发生 panic: %v", r)
		}
	}()

	// 6. 处理新增和更新
	for i := 0; i < sourceLen; i++ {
		sourceItem := sourceSlice.Index(i).Interface()
		sourceID := s.getIdFromEntity(sourceItem)
		targetItem, exists := targetMap[sourceID]

		if !exists {
			// 新增
			if err := tx.Table(s.tableName).Create(sourceItem).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("插入数据 %d 失败: %w", sourceID, err)
			}
			added++
		} else {
			// 检查是否有变化
			if s.hasChanged != nil && s.hasChanged(targetItem, sourceItem) {
				// 更新 - 使用 Updates 方法
				if err := tx.Table(s.tableName).Where("Id = ? OR id = ?", sourceID, sourceID).Updates(sourceItem).Error; err != nil {
					tx.Rollback()
					return fmt.Errorf("更新数据 %d 失败: %w", sourceID, err)
				}
				updated++
			}
			// 标记为已处理
			delete(targetMap, sourceID)
		}
	}

	// 7. 处理删除（targetDB 中存在但 SourceDB 中不存在的）
	for id := range targetMap {
		if err := tx.Table(s.tableName).Where("Id = ? OR id = ?", id, id).Delete(reflect.New(s.entityType).Interface()).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("删除数据 %d 失败: %w", id, err)
		}
		deleted++
	}

	// 8. 提交事务
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	duration := time.Since(startTime)
	zap.S().Infof("%s 表同步完成 - 新增: %d, 更新: %d, 删除: %d, 耗时: %v",
		s.serviceName, added, updated, deleted, duration)

	return nil
}

// getIdFromEntity 从实体中获取 ID
func (s *TableSyncService) getIdFromEntity(entity interface{}) int {
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	idField := v.FieldByName("Id")
	if !idField.IsValid() {
		// 尝试小写 id
		idField = v.FieldByName("id")
	}
	if idField.IsValid() && idField.CanInterface() {
		return int(idField.Int())
	}
	return 0
}

// StartDailySync 启动每日定时同步
func (s *TableSyncService) StartDailySync(ctx context.Context) error {
	go func() {
		now := time.Now()
		loc, _ := time.LoadLocation("Asia/Shanghai")
		today := now.In(loc)
		nextNoon := time.Date(today.Year(), today.Month(), today.Day(), 12, 0, 0, 0, loc)
		if now.After(nextNoon) {
			nextNoon = nextNoon.Add(24 * time.Hour)
		}

		waitDuration := nextNoon.Sub(now)
		zap.S().Infof("%s 同步任务将在 %v 后首次执行（%s）", s.serviceName, waitDuration, nextNoon.Format("2006-01-02 15:04:05"))

		select {
		case <-time.After(waitDuration):
			if err := s.Sync(); err != nil {
				zap.S().Errorf("首次同步失败: %v", err)
			}
		case <-ctx.Done():
			return
		}

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := s.Sync(); err != nil {
					zap.S().Errorf("定时同步失败: %v", err)
				}
			case <-ctx.Done():
				zap.S().Infof("%s 同步任务已停止", s.serviceName)
				return
			}
		}
	}()

	return nil
}

// StartCronSync 启动基于 cron 表达式的定时同步
func (s *TableSyncService) StartCronSync(ctx context.Context, cfg *Config) error {
	expr := strings.TrimSpace(cfg.Schedule.Cron)
	if expr == "" {
		return fmt.Errorf("cron 表达式不能为空")
	}

	parts := strings.Fields(expr)
	var c *cron.Cron
	if len(parts) == 6 {
		c = cron.New(cron.WithSeconds())
	} else if len(parts) == 5 {
		c = cron.New()
	} else {
		return fmt.Errorf("无效的 cron 表达式格式，应为5位或6位: %s", expr)
	}

	entryID, err := c.AddFunc(expr, func() {
		zap.S().Infof("CRON 触发 %s 同步任务...", s.serviceName)
		if err := s.Sync(); err != nil {
			zap.S().Errorf("CRON 调度执行失败: %v", err)
		} else {
			zap.S().Infof("CRON 调度执行成功")
		}
	})
	if err != nil {
		return fmt.Errorf("解析 CRON 表达式失败: %w", err)
	}

	zap.S().Infof("CRON 任务已注册 (EntryID: %d, 表达式: %s)", entryID, expr)

	c.Start()
	zap.S().Info("CRON 调度器已启动")

	go func() {
		<-ctx.Done()
		zap.S().Info("接收到停止信号，正在停止 CRON 调度器...")
		stopCtx := c.Stop()
		<-stopCtx.Done()
		zap.S().Info("CRON 调度器已停止")
	}()

	return nil
}

// SyncNow 立即执行一次同步
func (s *TableSyncService) SyncNow() error {
	return s.Sync()
}

// ColumnSyncService T_COLUMN 表同步服务（使用通用服务）
type ColumnSyncService struct {
	*TableSyncService
}

// NewColumnSyncServiceWithDB 创建栏目同步服务实例
func NewColumnSyncServiceWithDB(sourceDB, targetDB *gorm.DB) *ColumnSyncService {
	hasChanged := func(old, new interface{}) bool {
		oldCol := old.(*models.TColumn)
		newCol := new.(*models.TColumn)
		return oldCol.Name != newCol.Name ||
			oldCol.SiteId != newCol.SiteId ||
			oldCol.ParentId != newCol.ParentId ||
			oldCol.Path != newCol.Path
	}

	service := NewTableSyncService(
		sourceDB,
		targetDB,
		models.TableNameTColumn,
		reflect.TypeOf(models.TColumn{}),
		hasChanged,
		"T_COLUMN",
	)

	return &ColumnSyncService{TableSyncService: service}
}

// SyncColumns 执行同步操作（保持向后兼容）
func (s *ColumnSyncService) SyncColumns() error {
	return s.Sync()
}

// Sync 重写同步方法，添加 parentId 的特殊处理
func (s *ColumnSyncService) Sync() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("同步任务正在运行中，请稍后再试")
	}
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	if s.sourceDB == nil {
		return fmt.Errorf("SourceDB 未初始化")
	}
	if s.targetDB == nil {
		return fmt.Errorf("targetDB 未初始化")
	}

	zap.S().Infof("开始同步 %s 表...", s.serviceName)
	startTime := time.Now()

	// 1. 从 SourceDB 读取所有栏目数据
	var sourceColumns []models.TColumn
	if err := s.sourceDB.Table(s.tableName).Find(&sourceColumns).Error; err != nil {
		return fmt.Errorf("从 SourceDB 读取数据失败: %w", err)
	}

	zap.S().Infof("从 SourceDB 读取到 %d 条数据", len(sourceColumns))

	// 2. 处理 parentId：如果原表是 null（在 Go 中表现为 0），则改为 1
	for i := range sourceColumns {
		if sourceColumns[i].ParentId == 0 {
			sourceColumns[i].ParentId = 1
		}
	}

	// 3. 从 targetDB 读取现有数据
	var targetColumns []models.TColumn
	if err := s.targetDB.Table(s.tableName).Find(&targetColumns).Error; err != nil {
		zap.S().Warnf("从 targetDB 读取数据失败（可能表不存在）: %v", err)
		targetColumns = []models.TColumn{}
	}

	// 4. 构建现有数据的映射（以 Id 为 key）
	targetMap := make(map[int]*models.TColumn)
	for i := range targetColumns {
		targetMap[targetColumns[i].Id] = &targetColumns[i]
	}

	// 5. 统计变更
	var added, updated, deleted int

	// 6. 开始事务
	tx := s.targetDB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("开启事务失败: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			zap.S().Errorf("同步过程发生 panic: %v", r)
		}
	}()

	// 7. 处理新增和更新
	for i := range sourceColumns {
		sourceCol := &sourceColumns[i]
		targetCol, exists := targetMap[sourceCol.Id]

		if !exists {
			// 新增
			if err := tx.Table(s.tableName).Create(sourceCol).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("插入数据 %d 失败: %w", sourceCol.Id, err)
			}
			added++
		} else {
			// 检查是否有变化
			if s.hasChanged != nil && s.hasChanged(targetCol, sourceCol) {
				// 更新 - 使用 Updates 方法
				if err := tx.Table(s.tableName).Where("Id = ? OR id = ?", sourceCol.Id, sourceCol.Id).Updates(sourceCol).Error; err != nil {
					tx.Rollback()
					return fmt.Errorf("更新数据 %d 失败: %w", sourceCol.Id, err)
				}
				updated++
			}
			// 标记为已处理
			delete(targetMap, sourceCol.Id)
		}
	}

	// 8. 处理删除（targetDB 中存在但 SourceDB 中不存在的）
	for id := range targetMap {
		if err := tx.Table(s.tableName).Where("Id = ? OR id = ?", id, id).Delete(&models.TColumn{}).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("删除数据 %d 失败: %w", id, err)
		}
		deleted++
	}

	// 9. 提交事务
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	duration := time.Since(startTime)
	zap.S().Infof("%s 表同步完成 - 新增: %d, 更新: %d, 删除: %d, 耗时: %v",
		s.serviceName, added, updated, deleted, duration)

	return nil
}

// StartDailySync 启动每日定时同步（保持向后兼容）
func (s *ColumnSyncService) StartDailySync(ctx context.Context) error {
	return s.TableSyncService.StartDailySync(ctx)
}

// StartCronSync 启动基于 cron 表达式的定时同步（保持向后兼容）
func (s *ColumnSyncService) StartCronSync(ctx context.Context, cfg *Config) error {
	return s.TableSyncService.StartCronSync(ctx, cfg)
}

// SiteSyncService T_SITE 表同步服务（使用通用服务）
type SiteSyncService struct {
	*TableSyncService
}

// NewSiteSyncServiceWithDB 创建站点同步服务实例
func NewSiteSyncServiceWithDB(sourceDB, targetDB *gorm.DB) *SiteSyncService {
	hasChanged := func(old, new interface{}) bool {
		oldSite := old.(*models.TSite)
		newSite := new.(*models.TSite)
		return oldSite.Name != newSite.Name ||
			oldSite.DomainName != newSite.DomainName
	}

	service := NewTableSyncService(
		sourceDB,
		targetDB,
		models.TableNameTSite,
		reflect.TypeOf(models.TSite{}),
		hasChanged,
		"T_SITE",
	)

	return &SiteSyncService{TableSyncService: service}
}

// SyncSites 执行同步操作（保持向后兼容）
func (s *SiteSyncService) SyncSites() error {
	return s.Sync()
}

// StartDailySync 启动每日定时同步（保持向后兼容）
func (s *SiteSyncService) StartDailySync(ctx context.Context) error {
	return s.TableSyncService.StartDailySync(ctx)
}

// StartCronSync 启动基于 cron 表达式的定时同步（保持向后兼容）
func (s *SiteSyncService) StartCronSync(ctx context.Context, cfg *Config) error {
	return s.TableSyncService.StartCronSync(ctx, cfg)
}

// SyncNow 立即执行一次同步（保持向后兼容）
func (s *SiteSyncService) SyncNow() error {
	return s.TableSyncService.SyncNow()
}
