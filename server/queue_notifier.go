package server

import (
	"context"

	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/queue"
)

// queueNotifier implements queue.Notifier interface.
type queueNotifier struct {
	dropHook OnMsgDropped
	sts      *statsManager
}

func (q *queueNotifier) notifyDropped(clientID string, msg *gmqtt.Message, err error) {
	zaplog.Warn("message dropped", zap.String("client_id", clientID), zap.Error(err))
	q.sts.messageDropped(msg.QoS, clientID, err)
	if q.dropHook != nil {
		q.dropHook(context.Background(), clientID, msg, err)
	}
}

func (q *queueNotifier) NotifyDropped(clientID string, elem *queue.Elem, err error) {
	if pub, ok := elem.MessageWithID.(*queue.Publish); ok {
		q.notifyDropped(clientID, pub.Message, err)
	} else {
		zaplog.Warn("message dropped", zap.String("client_id", clientID), zap.Error(err))
	}
}

func (q *queueNotifier) NotifyInflightAdded(clientID string, delta int) {
	if delta > 0 {
		q.sts.addInflight(clientID, uint64(delta))
	}
	if delta < 0 {
		q.sts.decInflight(clientID, uint64(-delta))
	}

}

func (q *queueNotifier) NotifyMsgQueueAdded(clientID string, delta int) {
	if delta > 0 {
		q.sts.addQueueLen(clientID, uint64(delta))
	}
	if delta < 0 {
		q.sts.decQueueLen(clientID, uint64(-delta))
	}
}
