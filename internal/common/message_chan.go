package common

// unbounded channel
type MessageChan struct {
	in     chan<- *OctopusEvent
	out    <-chan *OctopusEvent
	buffer []*OctopusEvent
}

func NewMessageChan(capacity int) *MessageChan {
	in := make(chan *OctopusEvent, capacity)
	out := make(chan *OctopusEvent, capacity)

	ch := &MessageChan{
		in:     in,
		out:    out,
		buffer: make([]*OctopusEvent, 0, capacity),
	}

	go func() {
		defer close(out)

	loop:
		for {
			val, ok := <-in
			if !ok {
				break loop
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
						ch.buffer = make([]*OctopusEvent, 0, capacity)
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

func (ch *MessageChan) In() chan<- *OctopusEvent {
	return ch.in
}

func (ch *MessageChan) Out() <-chan *OctopusEvent {
	return ch.out
}
