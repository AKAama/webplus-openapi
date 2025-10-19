package nsc

import (
	"errors"
	"fmt"
	"sync"
	"time"
	"webplus-openapi/pkg/util"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"go.uber.org/zap"
)

var (
	singleton *NatsSubClient
	once      sync.Once
)

type NatsSubClient struct {
	clientName string
	cfg        *NatsConfig
	nc         *nats.Conn
	js         nats.JetStreamContext
	mutex      sync.RWMutex
	aSub       *nats.Subscription
	subs       map[string]*nats.Subscription
}

func InitNats(clientName string, config *NatsConfig) error {
	zap.S().Info("***初始化NATS")
	var hasError error
	once.Do(func() {
		client := &NatsSubClient{
			clientName: clientName,
			cfg:        config,
			nc:         nil,
			subs:       make(map[string]*nats.Subscription),
		}
		defaultAccount, err := config.GetDefaultAccount()
		if err != nil {
			hasError = err
			return
		}
		if err := client.Connect(defaultAccount); err != nil {
			hasError = err
			return
		}
		singleton = client
	})
	return hasError
}
func (nsc *NatsSubClient) Connect(account *NatsAccount) error {
	if nsc.nc != nil {
		return nil
	}
	opt := nats.GetDefaultOptions()
	opt.Name = fmt.Sprintf("%s %s", util.GetVersion().AppName, util.GetVersion().Version)
	opt.User = account.UserName
	opt.Password = account.Password
	opt.Nkey = account.NKey
	opt.Url = nsc.cfg.Endpoint
	opt.NoCallbacksAfterClientClose = true
	opt.ReconnectWait = 2 * time.Second //重试等待2s
	opt.MaxReconnect = -1               //永远重试
	opt.AllowReconnect = true
	opt.ReconnectJitter = 500 * time.Millisecond
	opt.DisconnectedErrCB = func(conn *nats.Conn, err error) {
		zap.S().Debugf("*** 断开连接...%s ***", err.Error())
	}
	opt.ReconnectedCB = func(conn *nats.Conn) {
		zap.S().Debugf("*** 已重连 ***")
	}
	opt.ConnectedCB = func(conn *nats.Conn) {
		zap.S().Debugf("*** NATS 已连接 ***")
	}

	opt.SignatureCB = func(b []byte) ([]byte, error) {
		sk, err := nkeys.FromSeed(util.StringToBytes(account.Seed))
		if err != nil {
			return nil, err
		}
		return sk.Sign(b)
	}

	nc, err := opt.Connect()
	if err != nil {
		return err
	}
	nc.SetErrorHandler(func(conn *nats.Conn, sub *nats.Subscription, natsErr error) {
		switch {
		case errors.Is(natsErr, nats.ErrSlowConsumer):
			pms, _, pmsErr := sub.Pending()
			if pmsErr != nil {
				zap.S().Errorf("couldn't get pending messages pending messages: %v", err)
			} else {
				zap.S().Errorf("Falling behind with %d pending messages on subject %q.\n",
					pms, sub.Subject)
			}

		default:
			zap.S().Errorf("Nats 捕获错误: %v", natsErr)
		}
	})
	nsc.nc = nc
	return nil
}
func (nsc *NatsSubClient) Close() {
	if nsc.aSub != nil {
		_ = nsc.aSub.Unsubscribe()
	}
	if nsc.nc != nil {
		_ = nsc.nc.Drain()
		nsc.nc.Close()
		zap.S().Debugf("*** NATS 已经关闭 ***")
	}

}

func GetNatsClient() *NatsSubClient {
	if singleton == nil {
		zap.S().Fatal("无法使用nats，请先初始化nats")
	}
	return singleton
}

func (nsc *NatsSubClient) GetNatsConn() *nats.Conn {
	return nsc.nc
}
