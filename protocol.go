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
 * PDUs
 *----------------------------------------------------------------------------*/
type PDU struct {
	Type  int
	Value interface{}
}

const (
	IntegerT          = 2
	OctetStringT      = 4
	NullT             = 5
	ObjectIdentifierT = 6
	IpAddressT        = 64
	Counter32T        = 65
	Gauge32T          = 66
	TimeTicksT        = 67
	OpaqueT           = 68
	Counter64T        = 70
	NoSuchObjectT     = 128
	NoSuchInstanceT   = 129
	EndOfMibViewT     = 130
)

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
	CloseTransactionId      = 86
	RegisterTransactionId   = 47
	UnregisterTransactionId = 74
)

const (
	HeaderSize int = 20
)

type Message interface {
	MarshalBinary() ([]byte, error)
	UnmarshalBinary([]byte) (int, error)
	//TODO
	//WireSize() int
}

// Header .....................................................................

type Header struct {
	Version, Type, Flags, Reserved byte
	SessionId                      int32
	TransactionId                  int32
	PacketId                       int32
	PayloadLength                  int32
}

func (h Header) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, h)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (h *Header) UnmarshalBinary(buf []byte) (int, error) {
	r := bytes.NewReader(buf)
	begin := r.Len()
	err := binary.Read(r, binary.BigEndian, h)
	if err != nil {
		return begin - r.Len(), err
	}
	return begin - r.Len(), nil
}

// Response ...................................................................

type Response struct {
	Header Header
	ResponsePayload
}

func (m *Response) UnmarshalBinary(buf []byte) (int, error) {
	i := 0
	n, err := m.Header.UnmarshalBinary(buf)
	if err != nil {
		return i, err
	}
	i += n

	n, err = m.ResponsePayload.UnmarshalBinary(buf[i:])
	if err != nil {
		return i, err
	}
	i += n

	return i, nil
}

func (m Response) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	if _, err := marshalToBuf(buf, &m.Header); err != nil {
		return nil, err
	}
	if err := netMarshalMany(buf, m.SysUptime, m.Error, m.Index); err != nil {
		return nil, err
	}
	for _, v := range m.VarBindList {
		b, err := v.MarshalBinary()
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	return buf.Bytes(), nil
}

type ResponsePayload struct {
	SysUptime   int32
	Error       int16
	Index       int16
	VarBindList []VarBind
}

func (p *ResponsePayload) UnmarshalBinary(buf []byte) (int, error) {
	r := bytes.NewReader(buf)
	before := r.Len()

	i := 0
	n, err := netUnmarshalMany(r, &p.SysUptime, &p.Error, &p.Index)
	if err != nil {
		return i, err
	}
	i += n

	//TODO unmarshal var bind list

	return before - r.Len(), nil
}

func NoSuchObjectVarBind(oid Subtree) VarBind {
	var v VarBind
	v.Type = NoSuchObjectT
	v.Name = oid
	return v
}

func EndOfMibViewVarBind(oid Subtree) VarBind {
	var v VarBind
	v.Type = EndOfMibViewT
	v.Name = oid
	return v
}

func OctetStringVarBind(oid Subtree, s []byte) *VarBind {
	return &VarBind{
		Type: OctetStringT,
		Name: oid,
		Data: *NewOctetString(s),
	}
}

// VarBind

type VarBind struct {
	Type     int16
	Reserved int16
	Name     Subtree
	Data     interface{}
}

func (v VarBind) WireSize() int {

	sz := 4 + v.Name.WireSize()

	switch v.Type {
	case IntegerT:
		sz += 4
	case OctetStringT:
		s := v.Data.(OctetString)
		sz += 4 + len(s.Octets)
	case Gauge32T:
		sz += 4
	//TODO below not implemented
	case NullT:
	case ObjectIdentifierT:
	case IpAddressT:
	case Counter32T:
	case TimeTicksT:
	case OpaqueT:
	case Counter64T:
	case NoSuchObjectT:
	case NoSuchInstanceT:
	case EndOfMibViewT:
	}

	return sz
}

func (v VarBind) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := netMarshalMany(buf, v.Type, v.Reserved); err != nil {
		return nil, err
	}

	if _, err := marshalToBuf(buf, &v.Name); err != nil {
		return nil, err
	}

	switch v.Type {
	case IntegerT:
		i := v.Data.(int32)
		if err := netMarshal(buf, i); err != nil {
			return nil, err
		}
	case OctetStringT:
		s := v.Data.(OctetString)
		if _, err := marshalToBuf(buf, &s); err != nil {
			return nil, err
		}
	case Gauge32T:
		i := v.Data.(uint32)
		if err := netMarshal(buf, i); err != nil {
			return nil, err
		}
	//TODO below not implemented
	case NullT:
	case ObjectIdentifierT:
	case IpAddressT:
	case Counter32T:
	case TimeTicksT:
	case OpaqueT:
	case Counter64T:
	case NoSuchObjectT:
	case NoSuchInstanceT:
	case EndOfMibViewT:
	}

	return buf.Bytes(), nil
}

func (v *VarBind) UnmarshalBinary(buf []byte) (int, error) {
	r := bytes.NewReader(buf)
	before := r.Len()

	i := 0
	n, err := netUnmarshalMany(r, &v.Type, &v.Reserved)
	if err != nil {
		return i, err
	}
	i += n

	n, err = v.Name.UnmarshalBinary(buf[i:])
	if err != nil {
		return i, err
	}
	i += n

	r = bytes.NewReader(buf[i:])
	switch v.Type {
	case IntegerT:
		var x int32
		n, err := netUnmarshal(r, &x)
		if err != nil {
			return i, err
		}
		v.Data = x
		i += n
	case OctetStringT:
		var x OctetString
		n, err := x.UnmarshalBinary(buf[i:])
		if err != nil {
			return i, err
		}
		v.Data = x
		i += n
	case Gauge32T:
		var x uint32
		n, err := netUnmarshal(r, &x)
		if err != nil {
			return i, err
		}
		v.Data = x
		i += n
	//TODO below not implemented
	case NullT:
	case ObjectIdentifierT:
	case IpAddressT:
	case Counter32T:
	case TimeTicksT:
	case OpaqueT:
	case Counter64T:
	case NoSuchObjectT:
	case NoSuchInstanceT:
	case EndOfMibViewT:
	}

	return before - r.Len(), nil
}

func IntegerVarBind(oid Subtree, value int32) VarBind {
	var v VarBind
	v.Type = IntegerT
	v.Name = oid
	v.Data = value
	return v
}

func Gauge32VarBind(oid Subtree, value uint32) VarBind {
	var v VarBind
	v.Type = Gauge32T
	v.Name = oid
	v.Data = value
	return v
}

// Subtree ....................................................................

type Subtree struct {
	NSubid, Prefix, Zero, Reserved byte
	SubIdentifiers                 []int32
}

func (s Subtree) HasPrefix(p Subtree) bool {
	//TODO can be more efficient without string conv
	return strings.HasPrefix(s.String(), p.String())
}

func (s Subtree) GreaterThan(x Subtree) bool {
	//TODO can be more efficient without string conv
	return s.String() > x.String()
}

func (s Subtree) GreaterThanEq(x Subtree) bool {
	//TODO can be more efficient without string conv
	return s.String() >= x.String()
}

func (s Subtree) LessThan(x Subtree) bool {
	//TODO can be more efficient without string conv
	return s.String() < x.String()
}

func (s Subtree) LessThanEq(x Subtree) bool {
	//TODO can be more efficient without string conv
	return s.String() <= x.String()
}

func (s Subtree) WireSize() int {
	return 4 + len(s.SubIdentifiers)*4
}

func NewSubtree(oid string) (*Subtree, error) {
	t := &Subtree{}

	ids := strings.Split(oid, ".")
	t.NSubid = byte(len(ids))
	for _, x := range ids {
		i, err := strconv.ParseInt(x, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("bad id, must be oid format: %v", err)
		}
		t.SubIdentifiers = append(t.SubIdentifiers, int32(i))
	}

	return t, nil
}

func (s Subtree) String() string {
	str := strconv.Itoa(int(s.SubIdentifiers[0]))
	for _, x := range s.SubIdentifiers[1:] {
		str += "." + strconv.Itoa(int(x))
	}
	if s.Prefix != 0 {
		str = fmt.Sprintf("1.3.6.1.%d.%s", s.Prefix, str)
	}
	return str
}

func (s Subtree) MarshalBinary() ([]byte, error) {
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

func (s *Subtree) UnmarshalBinary(buf []byte) (int, error) {
	r := bytes.NewReader(buf)
	before := r.Len()

	if n, err := netUnmarshalMany(r,
		&s.NSubid, &s.Prefix, &s.Zero, &s.Reserved); err != nil {
		return n, err
	}
	//log.Printf("reading %d subids", int(s.NSubid))
	for i := 0; i < int(s.NSubid); i++ {
		var v int32
		if n, err := netUnmarshal(r, &v); err != nil {
			return n, err
		}
		s.SubIdentifiers = append(s.SubIdentifiers, v)
	}
	return before - r.Len(), nil
}

// OctetString ..........................................................

type OctetString struct {
	OctetStringLength int32
	Octets            []byte
}

func NewOctetString(s []byte) *OctetString {
	os := &OctetString{
		OctetStringLength: int32(len(s)),
	}
	//copy to be sure
	os.Octets = make([]byte, len(s))
	copy(os.Octets, s)
	os.Pad()
	return os
}

func (s *OctetString) Pad() int {
	r := len(s.Octets) % 4
	if r == 0 {
		return 0
	}
	if len(s.Octets) == 0 {
		return 4
	}
	n := 4 - r
	for i := 0; i < n; i++ {
		s.Octets = append(s.Octets, 0)
	}
	return n
}

func (s OctetString) MarshalBinary() ([]byte, error) {
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

func (s *OctetString) UnmarshalBinary(buf []byte) (int, error) {
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
	Header   Header
	Timeout  byte
	Reserved [3]byte
	Id       Subtree
	Desc     OctetString
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
	Header   Header
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
	m.Header.TransactionId = CloseTransactionId
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
	Header                                  Header
	Context                                 *OctetString
	Timeout, Priority, RangeSubid, Reserved byte
	Subtree                                 Subtree
	UpperBound                              *int32
}

func NewRegisterMessage(subtree string, context *string, upperBound *int32) (
	*RegisterMessage, error) {

	m := &RegisterMessage{}
	m.Header.Version = 1
	m.Header.Type = RegisterPDU
	m.Header.Flags = NetworkByteOrder
	m.Header.PayloadLength = 4
	m.Header.TransactionId = RegisterTransactionId
	m.Timeout = ConnectionTimeout //from agx.go
	m.Priority = BasePriority     //from agx.go
	if context != nil {
		m.Header.Flags |= NonDefaultContext
	}

	//context
	if context != nil {
		m.Context = NewOctetString([]byte(*context))
		m.Header.PayloadLength += 4 + int32(len(m.Context.Octets))
	}

	//subtree
	subtree_, err := NewSubtree(subtree)
	if err != nil {
		return nil, err
	}
	m.Subtree = *subtree_
	m.Header.PayloadLength += int32(4 + 4*m.Subtree.NSubid)

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
		m.Context = &OctetString{}
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
	m.Header.TransactionId = UnregisterTransactionId
	return m, nil
}

// get ........................................................................

type GetMessage struct {
	Header          Header
	Context         *OctetString
	SearchRangeList []Subtree
}

type GetNextMessage struct {
	GetMessage
}

const (
	SearchRangeListPaddingLength = 4
)

func (m *GetMessage) UnmarshalBinary(buf []byte) (int, error) {
	return m.unmarshalBinary(buf, true)
}

func (m *GetNextMessage) UnmarshalBinary(buf []byte) (int, error) {
	return m.GetMessage.unmarshalBinary(buf, true)
}

func (m *GetMessage) unmarshalBinary(buf []byte, padded bool) (int, error) {
	i := 0
	n, err := m.Header.UnmarshalBinary(buf)
	if err != nil {
		return i, nil
	}
	i += n

	if (m.Header.Flags & NonDefaultContext) != 0 {
		m.Context = &OctetString{}
		n, err = m.Context.UnmarshalBinary(buf[i:])
		if err != nil {
			return i, nil
		}
		i += n
	}

	for i < len(buf) {
		var t Subtree
		n, err = t.UnmarshalBinary(buf[i:])
		if err != nil {
			return i, nil
		}
		if padded {
			i += n + SearchRangeListPaddingLength
		}
		if t.NSubid == 0 {
			continue
		}
		m.SearchRangeList = append(m.SearchRangeList, t)
	}

	return i, nil
}

// set ........................................................................

type TestSetResult int16

const (
	TestSetNoError             = TestSetResult(0)
	TestSetGenError            = TestSetResult(5)
	TestSetNoAccess            = TestSetResult(6)
	TestSetWrongType           = TestSetResult(7)
	TestSetWrongLength         = TestSetResult(8)
	TestSetWrongEncoding       = TestSetResult(9)
	TestSetWrongValue          = TestSetResult(10)
	TestSetNoCreation          = TestSetResult(11)
	TestSetInconsistentValue   = TestSetResult(12)
	TestSetResourceUnavailable = TestSetResult(13)
	TestSetNotWritable         = TestSetResult(17)
	TestSetInconsistentName    = TestSetResult(18)
)

type SetMessage struct {
	Header      Header
	Context     *OctetString
	VarBindList []VarBind
}

func (m *SetMessage) UnmarshalBinary(buf []byte) (int, error) {
	i := 0
	n, err := m.Header.UnmarshalBinary(buf)
	if err != nil {
		return i, nil
	}
	i += n

	if (m.Header.Flags & NonDefaultContext) != 0 {
		m.Context = &OctetString{}
		n, err = m.Context.UnmarshalBinary(buf[i:])
		if err != nil {
			return i, nil
		}
		i += n
	}

	for i < int(m.Header.PayloadLength) {
		var vb VarBind
		n, err = vb.UnmarshalBinary(buf[i:])
		if err != nil {
			return i, nil
		}
		i += n
		m.VarBindList = append(m.VarBindList, vb)
	}
	return i, nil
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

func marshalToBuf(buf *bytes.Buffer, m Message) (int, error) {
	b, err := m.MarshalBinary()
	if err != nil {
		return 0, err
	}
	n, err := buf.Write(b)
	return n, err
}

func marshalToBufs(buf *bytes.Buffer, ms ...Message) (int, error) {
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
