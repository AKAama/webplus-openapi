package nsc

import (
	"fmt"
)

type NatsAccount struct {
	UserName string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
	NKey     string `json:"nkey" yaml:"nkey"`
	Seed     string `json:"seed" yaml:"seed"`
}

type NatsConfig struct {
	Endpoint           string                  `json:"endpoint" yaml:"endpoint"`
	Account            map[string]*NatsAccount `json:"account" yaml:"account"`
	DefaultAccountName string                  `json:"defaultAccountName" yaml:"defaultAccountName"`
	WebplusStreamName  string                  `json:"webplusStreamName" yaml:"webplusStreamName"`
	ConsumerName       string                  `json:"consumerName" yaml:"consumerName"`
	SubjectName        string                  `json:"subjectName" yaml:"subjectName"`
}

func (n *NatsConfig) Validate() error {
	if len(n.Account) == 0 {
		return fmt.Errorf("尚未定义账号")
	}
	if len(n.ConsumerName) == 0 {
		return fmt.Errorf("尚未定义消费者")
	}
	if len(n.SubjectName) == 0 {
		return fmt.Errorf("尚未定义主题")
	}
	return nil
}
func NewDefaultNatsConfig() *NatsConfig {
	return &NatsConfig{
		Endpoint:           "nats://127.0.0.1:4222",
		DefaultAccountName: "",
		Account:            make(map[string]*NatsAccount),
	}
}
func (n *NatsConfig) GetDefaultAccount() (*NatsAccount, error) {
	if n.DefaultAccountName == "" {
		return nil, fmt.Errorf("没有定义默认账号")
	}
	if a, ok := n.Account[n.DefaultAccountName]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("无法找到 %s 账号定义", n.DefaultAccountName)
}
