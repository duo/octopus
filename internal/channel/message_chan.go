package channel

import (
	"reflect"
	"time"

	"github.com/duo/octopus/internal/common"

	log "github.com/sirupsen/logrus"
)

const filterTimeout = 30 * time.Second

// unbounded channel
type MessageChan struct {
	in      chan<- *common.OctopusEvent
	out     <-chan *common.OctopusEvent
	buffer  []*common.OctopusEvent
	filters []Filter
}

func New(capacity int, filters []Filter) *MessageChan {
	in := make(chan *common.OctopusEvent, capacity)
	out := make(chan *common.OctopusEvent, capacity)

	ch := &MessageChan{
		in:      in,
		out:     out,
		buffer:  make([]*common.OctopusEvent, 0, capacity),
		filters: filters,
	}

	go func() {
		defer close(out)

	loop:
		for {
			val, ok := <-in
			if !ok {
				break loop
			}

			if len(ch.filters) > 0 {
				for _, f := range ch.filters {
					result := make(chan *common.OctopusEvent, 1)

					go func() {
						result <- ProcessFilter(f, val)
					}()

					select {
					case <-time.After(filterTimeout):
						log.Warnf("Failed to process %v for event: %+v", reflect.TypeOf(f), val)
						val = nil
					case ret := <-result:
						val = ret
					}

					if val == nil {
						break
					}
				}

				if val == nil {
					continue
				}
			}

			select {
			case out <- val:
				continue
			default:
			}

			ch.buffer = append(ch.buffer, val)
			for len(ch.buffer) > 0 {
				select {
				case val, ok := <-in:
					if !ok {
						break loop
					}
					ch.buffer = append(ch.buffer, val)

				case out <- ch.buffer[0]:
					ch.buffer = ch.buffer[1:]
					if len(ch.buffer) == 0 {
						ch.buffer = make([]*common.OctopusEvent, 0, capacity)
					}
				}
			}
		}

		for len(ch.buffer) > 0 {
			out <- ch.buffer[0]
			ch.buffer = ch.buffer[1:]
		}
	}()

	return ch
}

func (ch *MessageChan) In() chan<- *common.OctopusEvent {
	return ch.in
}

func (ch *MessageChan) Out() <-chan *common.OctopusEvent {
	return ch.out
}
