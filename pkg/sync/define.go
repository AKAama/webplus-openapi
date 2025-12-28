package sync

import "webplus-openapi/pkg/db"

type Config struct {
	SourceDB *db.Config      `json:"sourceDB" yaml:"sourceDB"` // 来源库（读取）
	TargetDB *db.Config      `json:"targetDB" yaml:"targetDB"` // 业务字典库（写入/存储状态）
	Schedule *ScheduleConfig `json:"schedule" yaml:"schedule"`
}

type ScheduleConfig struct {
	RunOnStart bool   `json:"runOnStart" yaml:"runOnStart"`
	Cron       string `json:"cron,omitempty" yaml:"cron,omitempty"`
}

type RecordData struct {
	DataId         string   `json:"dataId"`
	DataName       string   `json:"dataName"`
	SimilarData    []string `json:"similarData"`
	LastUpdateTime string   `json:"lastUpdateTime"`
	ParentId       string   `json:"parentId,omitempty"`
}

type ChangeInfo struct {
	TableName string                       `json:"table_name"`
	Timestamp int64                        `json:"timestamp"`
	Added     map[string]RecordData        `json:"added"`
	Updated   map[string]RecordData        `json:"updated"`
	Deleted   map[string]RecordData        `json:"deleted"`
	Summary   map[string]int               `json:"summary,omitempty"`
	Extra     map[string]map[string]string `json:"extra,omitempty"`
}

// ColumnInfo 栏目信息响应
type ColumnInfo struct {
	ColumnId       int    `json:"columnId" `      // 栏目ID
	ColumnName     string `json:"columnName" `    // 栏目名称
	ParentColumnId string `json:"parentColumnId"` // 父栏目ID
	Path           string `json:"path"`           // 栏目路径
	ColumnUrl      string `json:"columnUrl"`      // 栏目链接
	Status         string `json:"status"`         // 栏目状态
	Sort           int    `json:"sort"`           // 栏目排序
}

type StateStore interface {
	SaveCurrentData(table string, data map[string]RecordData) error
	LoadHistoryData(table string) (map[string]RecordData, error)
	SaveChangeHistory(changes ChangeInfo) error
}

type MultiTableMonitor struct {
	config *Config
	store  StateStore
}

type TableManager struct {
	tableName string
	lastMap   map[string]RecordData
}
