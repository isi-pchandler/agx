// Package agx provdes an AgentX API compliant with RFC 2741.
package agx

// This file contains protocol definitinos that are used when communicating
// with a master agent
// ~~~
// Copyright Ryan Goodfellow 2017 - All Rights Reserved
// GPLv3

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
)

/*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 * OIDs
 *----------------------------------------------------------------------------*/
type OID struct {
}

func (o OID) String() string {
	panic("not implemented")
}

/*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 * PDUs
 *----------------------------------------------------------------------------*/
type PDU struct {
	Type  int
	Value interface{}
}

const (
	Integer          = 2
	OctetString      = 4
	Null             = 5
	ObjectIdentifier = 6
	IpAddress        = 64
	Counter32        = 65
	Gauge32          = 66
	TimeTicks        = 67
	Opaque           = 68
	Counter64        = 70
	NoSuchObject     = 128
	NoSuchInstance   = 129
	EndOfMibView     = 130
)

func NewOctetString(value []byte) PDU {
	panic("not implemented")
}

/*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 * AgentX Protocol
 *----------------------------------------------------------------------------*/
const (
	OpenPDU            = 1
	ClosePDU           = 2
	RegisterPDU        = 3
	UnregisterPDU      = 4
	GetPDU             = 5
	GetNextPDU         = 6
	GetBulkPDU         = 7
	TestSetPDU         = 8
	CommitSetPDU       = 9
	UndoSetPDU         = 10
	CleanupSetPDU      = 11
	NotifyPDU          = 12
	PingPDU            = 13
	IndexAllocatePDU   = 14
	IndexDeallocatePDU = 15
	AddAgentCapsPDU    = 16
	RemoveAgentCapsPDU = 17
	ResponsePDU        = 18
)

const (
	InstanceRegistration = 0x01
	NewIndex             = 0x02
	AnyIndex             = 0x04
	NonDefaultContext    = 0x08
	NetworkByteOrder     = 0x10
)

const (
	HeaderSize int = 20
)

type AgentXMessage interface {
	MarshalBinary() ([]byte, error)
	UnmarshalBinary([]byte) (int, error)
}

// AgentXHeader ...............................................................

type AgentXHeader struct {
	Version, Type, Flags, Reserved byte
	SessionId                      int32
	TransactionId                  int32
	PacketId                       int32
	PayloadLength                  int32
}

func (h AgentXHeader) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, h)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (h *AgentXHeader) UnmarshalBinary(buf []byte) (int, error) {
	r := bytes.NewReader(buf)
	err := binary.Read(r, binary.BigEndian, h)
	if err != nil {
		return int(r.Size()) - r.Len(), err
	}
	return int(r.Size()) - r.Len(), nil
}

// AgentXResponse .............................................................

type AgentXResponse struct {
	Header AgentXHeader
	AgentXResponsePayload
}

type AgentXResponsePayload struct {
	SysUptime int32
	Error     int16
	Index     int16
}

func (p *AgentXResponsePayload) UnmarshalBinary(buf []byte) (int, error) {
	r := bytes.NewReader(buf)
	err := binary.Read(r, binary.BigEndian, p)
	if err != nil {
		return int(r.Size()) - r.Len(), err
	}
	return int(r.Size()) - r.Len(), nil
}

// AgentXSubtree ..............................................................

type AgentXSubtree struct {
	NSubid, Prefix, Zero, Reserved byte
	SubIdentifiers                 []int32
}

func (s AgentXSubtree) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := netMarshalMany(buf,
		s.NSubid, s.Prefix, s.Zero, s.Reserved); err != nil {
		return nil, err
	}
	for _, v := range s.SubIdentifiers {
		if err := netMarshal(buf, v); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (s *AgentXSubtree) UnmarshalBinary(buf []byte) (int, error) {
	r := bytes.NewReader(buf)

	if n, err := netUnmarshalMany(r,
		&s.NSubid, &s.Prefix, &s.Zero, &s.Reserved); err != nil {
		return n, err
	}
	for i := 0; i < int(s.NSubid); i++ {
		var v int32
		if n, err := netUnmarshal(r, &v); err != nil {
			return n, err
		}
		s.SubIdentifiers = append(s.SubIdentifiers, v)
	}
	return int(r.Size()) - r.Len(), nil
}

// AgentXOctetString ..........................................................

type AgentXOctetString struct {
	OctetStringLength int32
	Octets            []byte
}

func (s *AgentXOctetString) Pad() int {
	r := len(s.Octets) % 4
	if r == 0 {
		return 0
	}
	n := 4 - r
	for i := 0; i < n; i++ {
		s.Octets = append(s.Octets, 0)
	}
	return n
}

func (s AgentXOctetString) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	s.Pad()

	if err := netMarshal(buf, s.OctetStringLength); err != nil {
		return nil, err
	}
	if _, err := buf.Write(s.Octets); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (s *AgentXOctetString) UnmarshalBinary(buf []byte) (int, error) {
	r := bytes.NewReader(buf)
	if _, err := netUnmarshal(r, &s.OctetStringLength); err != nil {
		return 0, err
	}
	for i := 0; i < int(s.OctetStringLength); i++ {
		var v byte
		if _, err := netUnmarshal(r, &v); err != nil {
			return i + 4, err
		}
		s.Octets = append(s.Octets, v)
	}
	padding := s.Pad()
	return 4 + int(s.OctetStringLength) + padding, nil
}

// open ......................................................................

type OpenMessage struct {
	Header   AgentXHeader
	Timeout  byte
	Reserved [3]byte
	Id       AgentXSubtree
	Desc     AgentXOctetString
}

func NewOpenMessage(id, descr *string) (*OpenMessage, error) {
	m := &OpenMessage{}
	m.Header.Version = 1
	m.Header.Type = OpenPDU
	m.Header.Flags = NetworkByteOrder
	m.Header.PayloadLength = 4
	m.Timeout = 5

	if id != nil {
		ids := strings.Split(*id, ".")
		m.Id.NSubid = byte(len(ids))
		m.Header.PayloadLength += int32(4 + 4*m.Id.NSubid)
		for _, x := range ids {
			i, err := strconv.ParseInt(x, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("bad id, must be oid format: %v", err)
			}
			m.Id.SubIdentifiers = append(m.Id.SubIdentifiers, int32(i))
		}
	}

	if descr != nil {
		bs := []byte(*descr)
		m.Desc.OctetStringLength = int32(len(bs))
		m.Desc.Octets = bs
		padlen := m.Desc.Pad()
		m.Header.PayloadLength += int32(4 + len(bs) + padlen)
	}

	return m, nil
}

func (m OpenMessage) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)

	if _, err := marshalToBuf(buf, &m.Header); err != nil {
		return nil, err
	}
	if err := netMarshalMany(buf, m.Timeout, m.Reserved); err != nil {
		return nil, err
	}
	if _, err := marshalToBufs(buf, &m.Id, &m.Desc); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (m *OpenMessage) UnmarshalBinary(buf []byte) (int, error) {
	i := 0

	n, err := m.Header.UnmarshalBinary(buf)
	if err != nil {
		return i, err
	}
	i += n

	r := bytes.NewReader(buf[i:])
	if _, err = netUnmarshal(r, &m.Timeout); err != nil {
		return i, err
	}
	i += 4

	n, err = m.Id.UnmarshalBinary(buf[i:])
	if err != nil {
		return i, err
	}
	i += n

	n, err = m.Desc.UnmarshalBinary(buf[i:])
	if err != nil {
		return i, err
	}
	i += n

	return i, nil
}

// close ......................................................................

type CloseMessage struct {
	Header   AgentXHeader
	Reason   byte
	Reserved [3]byte
}

func NewCloseMessage(reason byte, sessionId int32) *CloseMessage {
	m := &CloseMessage{}
	m.Header.Version = 1
	m.Header.Type = ClosePDU
	m.Header.Flags = NetworkByteOrder
	m.Header.PayloadLength = 4
	m.Header.SessionId = sessionId
	m.Header.PacketId = 1
	m.Header.TransactionId = 0
	m.Reason = reason
	return m
}

func (m CloseMessage) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)

	if _, err := marshalToBuf(buf, &m.Header); err != nil {
		return nil, err
	}
	if err := netMarshalMany(buf, m.Reason, m.Reserved); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *CloseMessage) UnmarshalBinary(buf []byte) (int, error) {
	i := 0
	n, err := m.Header.UnmarshalBinary(buf)
	if err != nil {
		return i, err
	}
	i += n

	r := bytes.NewReader(buf[i:])
	if _, err := netUnmarshal(r, &m.Reason); err != nil {
		return i, err
	}
	i += 4

	return i, nil
}

const (
	CloseReasonOther         byte = 1
	CloseReasonParseError         = 2
	CloseReasonProtocolError      = 3
	CloseReasonTimeouts           = 4
	CloseReasonShutdown           = 5
	CloseReasonByManaget          = 6
)

// register ...................................................................

type RegisterMessage struct {
	Header                                  AgentXHeader
	Context                                 *AgentXOctetString
	Timeout, Priority, RangeSubid, Reserved byte
	Subtree                                 AgentXSubtree
	UpperBound                              *int32
}

func NewRegisterMessage(subtree string, context *string, upperBound *int32) (
	*RegisterMessage, error) {

	m := &RegisterMessage{}
	m.Header.Version = 1
	m.Header.Type = RegisterPDU
	m.Header.Flags = NetworkByteOrder
	m.Header.PayloadLength = 4
	m.Timeout = ConnectionTimeout //from agx.go
	m.Priority = BasePriority     //from agx.go
	if context != nil {
		m.Header.Flags |= NonDefaultContext
	}

	//context
	if context != nil {
		m.Context = &AgentXOctetString{}
		bs := []byte(*context)
		m.Context.OctetStringLength = int32(len(bs))
		m.Context.Octets = bs
		padlen := m.Context.Pad()
		m.Header.PayloadLength += int32(4 + len(bs) + padlen)
	}

	//subtree
	ids := strings.Split(subtree, ".")
	m.Subtree.NSubid = byte(len(ids))
	m.Header.PayloadLength += int32(4 + 4*m.Subtree.NSubid)
	for _, x := range ids {
		i, err := strconv.ParseInt(x, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("bad id, must be oid format: %v", err)
		}
		m.Subtree.SubIdentifiers = append(m.Subtree.SubIdentifiers, int32(i))
	}

	//upper bound
	if upperBound != nil {
		m.UpperBound = new(int32)
		m.Header.PayloadLength += 4
		m.UpperBound = upperBound
	}

	return m, nil

}

func (m RegisterMessage) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)

	if _, err := marshalToBuf(buf, &m.Header); err != nil {
		return nil, err
	}

	if m.Context != nil {
		if _, err := marshalToBuf(buf, m.Context); err != nil {
			return nil, err
		}
	}

	if err := netMarshalMany(buf,
		m.Timeout, m.Priority, m.RangeSubid, m.Reserved); err != nil {
		return nil, err
	}

	if _, err := marshalToBuf(buf, &m.Subtree); err != nil {
		return nil, err
	}

	if m.UpperBound != nil {
		if err := netMarshal(buf, *m.UpperBound); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func (m *RegisterMessage) UnmarshalBinary(buf []byte) (int, error) {
	i := 0
	n, err := m.Header.UnmarshalBinary(buf)
	if err != nil {
		return i, nil
	}
	i += n

	if (m.Header.Flags & NonDefaultContext) != 0 {
		m.Context = &AgentXOctetString{}
		n, err = m.Context.UnmarshalBinary(buf[i:])
		if err != nil {
			return i, nil
		}
		i += n
	}

	rd := bytes.NewReader(buf[i:])
	n, err = netUnmarshalMany(rd,
		&m.Timeout, &m.Priority, &m.RangeSubid, &m.Reserved)
	if err != nil {
		return i, err
	}
	i += n

	n, err = m.Subtree.UnmarshalBinary(buf[i:])
	if err != nil {
		return i, nil
	}
	i += n

	if m.RangeSubid != 0 {
		r := bytes.NewReader(buf[i:])
		m.UpperBound = new(int32)
		if _, err := netUnmarshal(r, &m.UpperBound); err != nil {
			return i, err
		}
		i++
	}

	return i, nil
}

// unregister .................................................................

func NewUnregisterMessage(subtree string, context *string, upperBound *int32) (
	*RegisterMessage, error) {
	m, err := NewRegisterMessage(subtree, context, upperBound)
	if err != nil {
		return nil, err
	}
	m.Header.Type = UnregisterPDU
	return m, nil
}

// helpers ====================================================================
func netMarshal(w io.Writer, data interface{}) error {
	return binary.Write(w, binary.BigEndian, data)
}

func netMarshalMany(w io.Writer, items ...interface{}) error {
	for _, x := range items {
		err := netMarshal(w, x)
		if err != nil {
			return err
		}
	}
	return nil
}

func netUnmarshal(r *bytes.Reader, data interface{}) (int, error) {
	before := r.Len()
	err := binary.Read(r, binary.BigEndian, data)
	return before - r.Len(), err
}

func netUnmarshalMany(r *bytes.Reader, items ...interface{}) (int, error) {
	n := 0
	for _, x := range items {
		m, err := netUnmarshal(r, x)
		if err != nil {
			return n, err
		}
		n += m
	}
	return n, nil
}

func marshalToBuf(buf *bytes.Buffer, m AgentXMessage) (int, error) {
	b, err := m.MarshalBinary()
	if err != nil {
		return 0, err
	}
	n, err := buf.Write(b)
	return n, err
}

func marshalToBufs(buf *bytes.Buffer, ms ...AgentXMessage) (int, error) {
	n := 0
	for _, m := range ms {
		m, err := marshalToBuf(buf, m)
		if err != nil {
			return n, err
		}
		n += m
	}
	return n, nil
}
