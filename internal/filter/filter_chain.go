package filter

import "github.com/duo/octopus/internal/common"

type EventFilter interface {
	Apply(event *common.OctopusEvent) *common.OctopusEvent
}

type EventFilterChain struct {
	Filters []EventFilter
}

func (c EventFilterChain) Apply(event *common.OctopusEvent) *common.OctopusEvent {
	for _, filter := range c.Filters {
		event = filter.Apply(event)
	}
	return event
}

func NewEventFilterChain(filters ...EventFilter) EventFilterChain {
	return EventFilterChain{append(([]EventFilter)(nil), filters...)}
}
