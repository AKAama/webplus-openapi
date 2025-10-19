package recover

import (
	"sync"

	"gorm.io/gorm"
)

// ArticleRef 简化结构，用于获取ID
type ArticleRef struct {
	ID       string `gorm:"column:id"`        // 对应Article.ArticleId
	SiteID   string `gorm:"column:site_id"`   // 对应Article.SiteId
	ColumnID string `gorm:"column:column_id"` // 对应Article.PublishColumnId
}

// Params RecoverParams 恢复参数
type Params struct {
	SiteID         string `json:"site_id"`          // 站点ID，空字符串表示所有站点
	ColumnID       string `json:"column_id"`        // 栏目ID，空字符串表示所有栏目
	BatchSize      int    `json:"batch_size"`       // 批次大小
	Concurrency    int    `json:"concurrency"`      // 并发数，默认为CPU核心数
	WorkerPoolSize int    `json:"worker_pool_size"` // Worker池大小，默认为并发数的2倍
}

// BatchResult 处理结果统计结构
type BatchResult struct {
	ProcessedCount int // 实际处理的文章数
	SkippedCount   int // 跳过的文章数
	ErrorCount     int // 处理失败的文章数
}

// ProcessResult 单篇文章处理结果
type ProcessResult struct {
	Status string // "processed", "skipped", 或错误信息
}

// ConcurrentBatch 并发批次处理结构
type ConcurrentBatch struct {
	ID       int          // 批次ID
	Articles []ArticleRef // 文章列表
	Result   *BatchResult // 处理结果
	Error    error        // 错误信息
}

// ProgressTracker 进度跟踪器
type ProgressTracker struct {
	TotalBatches     int
	CompletedBatches int
	ProcessedCount   int
	SkippedCount     int
	ErrorCount       int
	mu               sync.RWMutex
}

// Service 数据恢复服务
type Service struct {
	db          *gorm.DB
	manager     *Manager
	badgerStore BadgerStore
}

// Manager Webplus管理器
type Manager struct {
	cfg *Config
}

// ArticleRepository 文章数据访问层
type ArticleRepository struct {
	db *gorm.DB
}

// ArticleService 文章业务逻辑层
type ArticleService struct {
	repo        *ArticleRepository
	badgerStore BadgerStore
}

// NewArticleRepository 创建文章数据访问层
func NewArticleRepository(db *gorm.DB) *ArticleRepository {
	return &ArticleRepository{db: db}
}

// NewArticleService 创建文章业务逻辑层
func NewArticleService(repo *ArticleRepository, badgerStore BadgerStore) *ArticleService {
	return &ArticleService{
		repo:        repo,
		badgerStore: badgerStore,
	}
}

// NewProgressTracker 创建进度跟踪器
func NewProgressTracker(totalBatches int) *ProgressTracker {
	return &ProgressTracker{
		TotalBatches: totalBatches,
	}
}

// UpdateProgress 更新进度
func (pt *ProgressTracker) UpdateProgress(result *BatchResult) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.CompletedBatches++
	pt.ProcessedCount += result.ProcessedCount
	pt.SkippedCount += result.SkippedCount
	pt.ErrorCount += result.ErrorCount
}

// GetProgress 获取当前进度
func (pt *ProgressTracker) GetProgress() (int, int, int, int, int) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	return pt.CompletedBatches, pt.TotalBatches, pt.ProcessedCount, pt.SkippedCount, pt.ErrorCount
}
