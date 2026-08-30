// Package events 提供 License 事件的内存发布/订阅(Event Broker)。
//
// 职责:持久化到 license_events 表之后,Publish 把事件扇出到对应 Activation 的
// SSE 订阅者。不承担持久化 —— 持久化在 service 层同一事务内完成,
// 提交成功后才 Publish(事务一致性,§19)。
//
// 设计要点:
//   - 按 activation_id(展示 ID,ACT-*)路由;activation_id = ” 的事件为全局广播,
//     所有订阅者都能收到(如 version_policy.changed)
//   - 线程安全;订阅/取消订阅/发布均可并发
//   - 慢消费者直接丢弃(有持久化 Event Store 兜底,客户端重连后按 Last-Event-ID 补齐)
//   - 无 Goroutine 泄漏:订阅者必须 defer Unsubscribe;channel 由订阅者创建与关闭
package events

import (
	"sync"

	"github.com/DockOrae/DockOrae-Auth/internal/model"
)

// Broker 内存事件发布/订阅。
type Broker struct {
	mu     sync.Mutex
	byAct  map[string]map[chan *model.LicenseEvent]struct{} // activation_id → 订阅者
	global map[chan *model.LicenseEvent]struct{}            // 全局广播订阅者
}

// NewBroker 构造。
func NewBroker() *Broker {
	return &Broker{
		byAct:  make(map[string]map[chan *model.LicenseEvent]struct{}),
		global: make(map[chan *model.LicenseEvent]struct{}),
	}
}

// Subscribe 订阅某 Activation 的事件(activationID 为 ACT-* 展示 ID)。
// 返回带缓冲的 channel;调用方负责 defer Unsubscribe。
func (b *Broker) Subscribe(activationID string) chan *model.LicenseEvent {
	ch := make(chan *model.LicenseEvent, 64)
	b.mu.Lock()
	defer b.mu.Unlock()
	if activationID == "" {
		b.global[ch] = struct{}{}
		return ch
	}
	m := b.byAct[activationID]
	if m == nil {
		m = make(map[chan *model.LicenseEvent]struct{})
		b.byAct[activationID] = m
	}
	m[ch] = struct{}{}
	return ch
}

// Unsubscribe 取消订阅(幂等)。
func (b *Broker) Unsubscribe(activationID string, ch chan *model.LicenseEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if activationID == "" {
		delete(b.global, ch)
		return
	}
	if m := b.byAct[activationID]; m != nil {
		delete(m, ch)
		if len(m) == 0 {
			delete(b.byAct, activationID)
		}
	}
}

// Publish 发布事件:广播给目标 Activation 的订阅者 + 全局订阅者。
// 慢消费者(缓冲满)直接丢弃 —— 客户端重连后通过 Last-Event-ID 从持久化 Store 补齐。
func (b *Broker) Publish(ev *model.LicenseEvent) {
	if ev == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if m := b.byAct[ev.ActivationID]; m != nil {
		for ch := range m {
			select {
			case ch <- ev:
			default:
			}
		}
	}
	for ch := range b.global {
		select {
		case ch <- ev:
		default:
		}
	}
}
