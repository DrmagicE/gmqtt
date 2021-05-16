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
	cli      *client
}

// defaultNotifier is used to init the notifier when using a persistent session store (e.g redis) which can load session data
// while bootstrapping.
func defaultNotifier(dropHook OnMsgDropped, sts *statsManager, clientID string) *queueNotifier {
	return &queueNotifier{
		dropHook: dropHook,
		sts:      sts,
		cli:      &client{opts: &ClientOptions{ClientID: clientID}, status: Connected + 1},
	}
}

func (q *queueNotifier) notifyDropped(msg *gmqtt.Message, err error) {
	cid := q.cli.opts.ClientID
	zaplog.Warn("message dropped", zap.String("client_id", cid), zap.Error(err))
	q.sts.messageDropped(msg.QoS, q.cli.opts.ClientID, err)
	if q.dropHook != nil {
		q.dropHook(context.Background(), cid, msg, err)
	}
}

func (q *queueNotifier) NotifyDropped(elem *queue.Elem, err error) {
	cid := q.cli.opts.ClientID
	if err == queue.ErrDropExpiredInflight && q.cli.IsConnected() {
		q.cli.pl.release(elem.ID())
	}
	if pub, ok := elem.MessageWithID.(*queue.Publish); ok {
		q.notifyDropped(pub.Message, err)
	} else {
		zaplog.Warn("message dropped", zap.String("client_id", cid), zap.Error(err))
	}
}

func (q *queueNotifier) NotifyInflightAdded(delta int) {
	cid := q.cli.opts.ClientID
	if delta > 0 {
		q.sts.addInflight(cid, uint64(delta))
	}
	if delta < 0 {
		q.sts.decInflight(cid, uint64(-delta))
	}

}

func (q *queueNotifier) NotifyMsgQueueAdded(delta int) {
	cid := q.cli.opts.ClientID
	if delta > 0 {
		q.sts.addQueueLen(cid, uint64(delta))
	}
	if delta < 0 {
		q.sts.decQueueLen(cid, uint64(-delta))
	}
}
