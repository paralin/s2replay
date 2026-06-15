package s2replay

import (
	"io"

	"github.com/paralin/s2replay/protocol"
)

// NextMessage returns the next decoded packet or user message. It unwraps
// DEM_Packet, DEM_SignonPacket, and DEM_FullPacket command payloads, routes
// inner ids through generated dispatch, and updates the clock from ServerInfo.
func (p *Parser) NextMessage() (*Message, error) {
	for len(p.pending) == 0 {
		cmd, err := p.Next()
		if err != nil {
			return nil, err
		}
		if err := p.queueCommandMessages(cmd); err != nil {
			return nil, err
		}
	}

	m := p.pending[0]
	copy(p.pending, p.pending[1:])
	p.pending = p.pending[:len(p.pending)-1]

	if serverInfo, ok := m.Payload.(*protocol.CSVCMsg_ServerInfo); ok {
		p.clock.SetInterval(float64(serverInfo.GetTickInterval()))
		m.GameTime = p.clock.GameTime()
	}
	return m, nil
}

func (p *Parser) queueCommandMessages(cmd *Command) error {
	decoded, ok, err := decodeDemoCommand(int32(cmd.Kind), cmd.Payload)
	if err != nil || !ok {
		return err
	}

	switch msg := decoded.msg.(type) {
	case *protocol.CDemoPacket:
		return p.queuePacketMessages(cmd.Tick, msg.GetData())
	case *protocol.CDemoFullPacket:
		if packet := msg.GetPacket(); packet != nil {
			return p.queuePacketMessages(cmd.Tick, packet.GetData())
		}
	}
	return nil
}

func (p *Parser) queuePacketMessages(tick uint32, payload []byte) error {
	r := newPacketReader(payload)
	for r.bitsRemaining() > 8 {
		kind, err := r.readUBitVar()
		if err != nil {
			return err
		}
		size, err := r.readUvarint32()
		if err != nil {
			return err
		}
		buf, err := r.readBytes(int(size))
		if err != nil {
			return err
		}

		decoded, ok, err := decodePacketMessage(int32(kind), buf)
		if err != nil || !ok {
			return err
		}
		p.appendMessage(tick, decoded)

		if user, ok := decoded.msg.(*protocol.CSVCMsg_UserMessage); ok {
			userDecoded, ok, err := decodeUserMessage(user.GetMsgType(), user.GetMsgData())
			if err != nil || !ok {
				return err
			}
			p.appendMessage(tick, userDecoded)
		}
	}
	return nil
}

func (p *Parser) appendMessage(tick uint32, decoded decodedMessage) {
	p.pending = append(p.pending, &Message{
		Kind:     decoded.kind,
		Name:     decoded.name,
		Tick:     tick,
		GameTime: p.clock.GameTime(),
		Payload:  decoded.msg,
	})
}

// NextDamage returns the next decoded Deadlock damage event.
func (p *Parser) NextDamage() (DamageEvent, error) {
	for {
		m, err := p.NextMessage()
		if err != nil {
			return DamageEvent{}, err
		}
		if ev, ok := m.DamageEvent(); ok {
			return ev, nil
		}
	}
}

// CollectDamage reads up to limit damage events. A non-positive limit reads the
// whole demo.
func (p *Parser) CollectDamage(limit int) ([]DamageEvent, error) {
	var events []DamageEvent
	for limit <= 0 || len(events) < limit {
		ev, err := p.NextDamage()
		if err == io.EOF {
			return events, nil
		}
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}
