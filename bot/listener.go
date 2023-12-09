package bot

import (
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"sort"
	"sync"
)

type Events struct {
	generic  []GenericHandler // for every packet
	hLock    sync.Mutex
	handlers map[uint32][]any // for specific packet id only
	tickers  []TickHandler
}

func AddListener[T packet.Packet](c *Client, listeners PacketHandler[T]) {
	e := c.events
	var t T
	id := t.ID()

	if s, ok := e.handlers[id]; !ok {
		e.handlers[id] = []any{listeners}
	} else {
		e.handlers[id] = append(s, listeners)
		sort.SliceStable(e.handlers[id], func(i, j int) bool {
			return e.handlers[id][i].(PacketHandler[T]).Priority > e.handlers[id][j].(PacketHandler[T]).Priority
		})
	}
}

// AddGeneric adds listeners like AddListener, but the packet ID is ignored.
// Generic listener is always called before specific packet listener.
func (e *Events) AddGeneric(listeners ...GenericHandler) {
	e.generic = append(e.generic, listeners...)
	sortGenericHandlers(e.generic)
}
func (e *Events) AddTicker(tickers ...TickHandler) {
	e.tickers = append(e.tickers, tickers...)
	sortTickHandlers(e.tickers)
}

type (
	PacketHandler[T any] struct {
		Priority int
		F        func(client *Client, p T) error
	}
	GenericHandler struct {
		ID       uint32
		Priority int
		F        func(client *Client, p packet.Packet) error
	}
	TickHandler struct {
		Priority int
		F        func(*Client) error
	}
)

func sortGenericHandlers(slice []GenericHandler) {
	sort.SliceStable(slice, func(i, j int) bool {
		return slice[i].Priority > slice[j].Priority
	})
}
func sortTickHandlers(slice []TickHandler) {
	sort.SliceStable(slice, func(i, j int) bool {
		return slice[i].Priority > slice[j].Priority
	})
}
