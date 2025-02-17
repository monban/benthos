package batcher

import (
	"context"
	"sync"
	"time"

	"github.com/benthosdev/benthos/v4/internal/batch/policy"
	"github.com/benthosdev/benthos/v4/internal/component"
	"github.com/benthosdev/benthos/v4/internal/component/input"
	"github.com/benthosdev/benthos/v4/internal/component/metrics"
	"github.com/benthosdev/benthos/v4/internal/log"
	"github.com/benthosdev/benthos/v4/internal/message"
	"github.com/benthosdev/benthos/v4/internal/shutdown"
	"github.com/benthosdev/benthos/v4/internal/transaction"
)

// Impl wraps an input with a batch policy.
type Impl struct {
	stats metrics.Type
	log   log.Modular

	child   input.Streamed
	batcher *policy.Batcher

	messagesOut chan message.Transaction

	shutSig *shutdown.Signaller
}

// New creates a new Batcher around an input.
func New(batcher *policy.Batcher, child input.Streamed, log log.Modular, stats metrics.Type) input.Streamed {
	b := Impl{
		stats:       stats,
		log:         log,
		child:       child,
		batcher:     batcher,
		messagesOut: make(chan message.Transaction),
		shutSig:     shutdown.NewSignaller(),
	}
	go b.loop()
	return &b
}

//------------------------------------------------------------------------------

func (m *Impl) loop() {
	defer func() {
		go func() {
			select {
			case <-m.shutSig.CloseNowChan():
				_ = m.child.WaitForClose(0)
				_ = m.batcher.WaitForClose(0)
			case <-m.shutSig.HasClosedChan():
			}
		}()

		m.child.CloseAsync()
		_ = m.child.WaitForClose(shutdown.MaximumShutdownWait())

		m.batcher.CloseAsync()
		_ = m.batcher.WaitForClose(shutdown.MaximumShutdownWait())

		close(m.messagesOut)
		m.shutSig.ShutdownComplete()
	}()

	var nextTimedBatchChan <-chan time.Time
	if tNext := m.batcher.UntilNext(); tNext >= 0 {
		nextTimedBatchChan = time.After(tNext)
	}

	pendingTrans := []*transaction.Tracked{}
	pendingAcks := sync.WaitGroup{}

	flushBatchFn := func() {
		sendMsg := m.batcher.Flush()
		if sendMsg == nil {
			return
		}

		resChan := make(chan error)
		select {
		case m.messagesOut <- message.NewTransaction(sendMsg, resChan):
		case <-m.shutSig.CloseNowChan():
			return
		}

		pendingAcks.Add(1)
		go func(rChan <-chan error, aggregatedTransactions []*transaction.Tracked) {
			defer pendingAcks.Done()

			select {
			case <-m.shutSig.CloseNowChan():
				return
			case res, open := <-rChan:
				if !open {
					return
				}
				closeNowCtx, done := m.shutSig.CloseNowCtx(context.Background())
				for _, c := range aggregatedTransactions {
					if err := c.Ack(closeNowCtx, res); err != nil {
						done()
						return
					}
				}
				done()
			}
		}(resChan, pendingTrans)
		pendingTrans = nil
	}

	defer func() {
		// Final flush of remaining documents.
		m.log.Debugln("Flushing remaining messages of batch.")
		flushBatchFn()

		// Wait for all pending acks to resolve.
		m.log.Debugln("Waiting for pending acks to resolve before shutting down.")
		pendingAcks.Wait()
		m.log.Debugln("Pending acks resolved.")
	}()

	for {
		if nextTimedBatchChan == nil {
			if tNext := m.batcher.UntilNext(); tNext >= 0 {
				nextTimedBatchChan = time.After(tNext)
			}
		}

		var flushBatch bool
		select {
		case tran, open := <-m.child.TransactionChan():
			if !open {
				// If we're waiting for a timed batch then we will respect it.
				if nextTimedBatchChan != nil {
					select {
					case <-nextTimedBatchChan:
					case <-m.shutSig.CloseAtLeisureChan():
						return
					}
				}
				flushBatchFn()
				return
			}

			trackedTran := transaction.NewTracked(tran.Payload, tran.Ack)
			_ = trackedTran.Message().Iter(func(i int, p *message.Part) error {
				if m.batcher.Add(p) {
					flushBatch = true
				}
				return nil
			})
			pendingTrans = append(pendingTrans, trackedTran)
		case <-nextTimedBatchChan:
			flushBatch = true
			nextTimedBatchChan = nil
		case <-m.shutSig.CloseAtLeisureChan():
			return
		}

		if flushBatch {
			flushBatchFn()
		}
	}
}

// Connected returns true if the underlying input is connected.
func (m *Impl) Connected() bool {
	return m.child.Connected()
}

// TransactionChan returns the channel used for consuming messages from this
// buffer.
func (m *Impl) TransactionChan() <-chan message.Transaction {
	return m.messagesOut
}

// CloseAsync shuts down the Batcher and stops processing messages.
func (m *Impl) CloseAsync() {
	m.shutSig.CloseAtLeisure()
}

// WaitForClose blocks until the Batcher output has closed down.
func (m *Impl) WaitForClose(timeout time.Duration) error {
	go func() {
		<-time.After(timeout - time.Second)
		m.shutSig.CloseNow()
	}()
	select {
	case <-m.shutSig.HasClosedChan():
	case <-time.After(timeout):
		return component.ErrTimeout
	}
	return nil
}
