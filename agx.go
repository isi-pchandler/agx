// Package agx provdes an AgentX API compliant with RFC 2741.
package agx

// This file contains the agx API
// ~~~
// Copyright Ryan Goodfellow 2017 - All Rights Reserved
// GPLv3

import (
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strings"
)

/*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 * Connections
 *----------------------------------------------------------------------------*/
type Connection struct {
	//private members
	conn               net.Conn
	sessionId          int32
	registrations      []string
	closed             bool
	getHandlers        map[string]GetHandler
	getSubtreeHandlers map[string]GetSubtreeHandler
	testSetHandlers    map[string]TestSetHandler
	commitSetHandler   CommitSetHandler
	cleanupSetHandler  CleanupSetHandler

	//public members
	Closed chan bool
}

const (
	ConnectionTimeout = 10 //only wait 10 seconds the master agent to reply
	BasePriority      = 47 //the default priprity that is given to registrations
)

// Connect to an master agent using the provided id and description. The
// connection object that is returned holds the session information for the
// connection. This connection pointer is the basis for using most other
// functions in the agx API.
func Connect(id, descr *string) (*Connection, error) {
	log.Printf("connecting")

	//use the well known agentx unix socket (RFC2741~8.2)
	c := &Connection{}
	conn, err := net.Dial("unix", "/var/agentx/master")
	if err != nil {
		return nil, fmt.Errorf("error connecting to agentx: %v", err)
	}
	c.conn = conn
	c.Closed = make(chan bool)
	c.getHandlers = make(map[string]GetHandler)
	c.getSubtreeHandlers = make(map[string]GetSubtreeHandler)
	c.testSetHandlers = make(map[string]TestSetHandler)

	//try to open a new AgentX session with the master
	m, err := NewOpenMessage(id, descr)
	if err != nil {
		return nil, fmt.Errorf("error creating open message: %v", err)
	}
	hdr, buf, err := sendrecvMsg(m, c)

	//grab the response payload, extract and save the sessionId
	p := &ResponsePayload{}
	_, err = p.UnmarshalBinary(buf[HeaderSize:])
	if err != nil {
		log.Printf("error reading open response playload: %v", err)
		return nil, err
	}
	c.sessionId = hdr.SessionId

	log.Printf("agent entering read loop")

	go rootMessageHandler(c)

	return c, nil
}

// Disconnect from the master agent. Sends a close PDU to the master agent
// which will effectively end the session contained within the provided
// connection object pointer. The passed in connection will be useless after
// this call.
func (c *Connection) Disconnect() {
	log.Printf("disconnecting session %d", c.sessionId)

	//send the close PDU to the master
	msg := NewCloseMessage(CloseReasonShutdown, c.sessionId)
	err := sendMsg(msg, c)
	if err != nil {
		if err == io.EOF {
			//ok connection is aleady closed
		} else {
			log.Printf("error closing connection %v", err)
		}
	}
}

func (c *Connection) Register(oid string) error {
	return c.doRegister(oid, false)
}

func (c *Connection) Unregister(oid string) error {
	return c.doRegister(oid, true)
}

func (c *Connection) doRegister(oid string, unregister bool) error {

	var m *RegisterMessage
	var err error
	var context = ""
	if unregister {
		m, err = NewUnregisterMessage(oid, &context, nil)
	} else {
		m, err = NewRegisterMessage(oid, &context, nil)
	}
	m.Header.PacketId = int32(len(c.registrations))
	c.registrations = append(c.registrations, oid)

	m.Header.SessionId = c.sessionId
	if err != nil {
		return fmt.Errorf("failed creating registration message %v", err)
	}

	sendMsg(m, c)

	return nil
}

/*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 * Agents
 *----------------------------------------------------------------------------*/
type GetHandler func(oid Subtree) VarBind
type GetSubtreeHandler func(oid Subtree, next bool) VarBind
type TestSetHandler func(vars VarBind, sessionId int) TestSetResult
type CommitSetHandler func(sessionId int) CommitSetResult
type CleanupSetHandler func(sessionId int)

func (c *Connection) OnGet(oid string, f GetHandler) {
	c.getHandlers[oid] = f
}

func (c *Connection) OnGetSubtree(oid string, f GetSubtreeHandler) {
	c.getSubtreeHandlers[oid] = f
}

func (c *Connection) OnTestSet(oid string, f TestSetHandler) {
	c.testSetHandlers[oid] = f
}

func (c *Connection) OnCommitSet(f CommitSetHandler) {
	c.commitSetHandler = f
}

func (c *Connection) OnCleanupSet(f CleanupSetHandler) {
	c.cleanupSetHandler = f
}

// helper functions ===========================================================

func sendMsg(m Message, c *Connection) error {
	if c.closed {
		return io.EOF
	}
	buf, err := m.MarshalBinary()
	if err != nil {
		return fmt.Errorf("error marshalling message: %v", err)
	}

	_, err = c.conn.Write(buf)
	if err != nil {
		return fmt.Errorf("error sending message: %v", err)
	}
	return nil
}

func recvMsg(c *Connection) (*Header, []byte, error) {
	buf := make([]byte, 1024)
	n, err := c.conn.Read(buf)
	if err != nil {
		if err == io.EOF {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("error getting message response: %v", err)
	}

	hdr := &Header{}
	_, err = hdr.UnmarshalBinary(buf[:n])
	if err != nil {
		log.Printf("failure reading response header: %v", err)
		return nil, nil, fmt.Errorf("failure reading response header: %v", err)
	}
	return hdr, buf, nil
}

func sendrecvMsg(m Message, c *Connection) (*Header, []byte, error) {
	err := sendMsg(m, c)
	if err != nil {
		return nil, nil, err
	}
	return recvMsg(c)
}

func rootMessageHandler(c *Connection) {
	log.Printf("[rootMH] waiting for messages")

	for {
		hdr, buf, err := recvMsg(c)
		if err != nil {
			if err == io.EOF {
				log.Printf("[rootMH] master agent has closed connection")
				c.Closed <- true
				c.closed = true
				return
			}
			log.Printf("[rootMH] failure reading incommig message: %v", err)
			continue
		}

		switch hdr.Type {
		case ResponsePDU:
			switch hdr.TransactionId {
			case CloseTransactionId:
				handleCloseResponse(c, hdr, buf)
			case RegisterTransactionId:
				handleRegisterResponse(c, hdr, buf)
			case UnregisterTransactionId:
				handleUnregisterResponse(c, hdr, buf)
			}
		case GetPDU:
			handleGet(c, hdr, buf)
		case GetNextPDU:
			handleGetNext(c, hdr, buf)
		case TestSetPDU:
			handleTestSet(c, hdr, buf)
		case CommitSetPDU:
			handleCommitSet(c, hdr, buf)
		case CleanupSetPDU:
			handleCleanupSet(c, hdr, buf)
		default:
			log.Printf("[roogMH] unknown message type %d", hdr.Type)
		}
	}
}

func handleCloseResponse(c *Connection, h *Header, buf []byte) {
	log.Printf("[rootMH] recieved close response from server, ... exiting\n")
	//grab the response payload and check for errors
	p := &ResponsePayload{}
	_, err := p.UnmarshalBinary(buf[HeaderSize:])
	if err != nil {
		log.Printf("error reading close response playload: %v", err)
		c.conn.Close()
		return
	}
	if p.Error != 0 {
		log.Printf("Master agent reporeted error on close %d", p.Error)
	}

	//close the unix domain socket
	c.conn.Close()
	c.Closed <- true
	c.closed = true
}

func handleRegisterResponse(c *Connection, h *Header, buf []byte) {
	p := &ResponsePayload{}
	_, err := p.UnmarshalBinary(buf[HeaderSize:])
	if err != nil {
		log.Printf("error reading response playload: %v", err)
		return
	}

	if p.Error == 0 {
		log.Printf(
			"[rootMH] received registration confrimation for %s\n",
			c.registrations[h.PacketId])
	} else {
		log.Printf(
			"[rootMH] received registration failure for %s\n",
			c.registrations[h.PacketId])
	}
}

func handleUnregisterResponse(c *Connection, h *Header, buf []byte) {
	log.Printf("[rootMH] received unregistration confrimation for %s\n",
		c.registrations[h.PacketId])
}

// get handling ...............................................................

func handleGet(c *Connection, h *Header, buf []byte) {
	doHandleGet(c, h, buf, false)
}

func handleGetNext(c *Connection, h *Header, buf []byte) {
	doHandleGet(c, h, buf, true)
}

func doHandleGet(c *Connection, h *Header, buf []byte, next bool) {
	g := &GetNextMessage{}
	_, err := g.UnmarshalBinary(buf)
	if err != nil {
		log.Printf("[getnext] error unmarshalling GetNextPDU %v\n", err)
	}

	var r Response
	r.Header.Version = 1
	r.Header.Type = ResponsePDU
	r.Header.Flags = h.Flags & NetworkByteOrder
	r.Header.SessionId = c.sessionId
	r.Header.TransactionId = h.TransactionId
	r.Header.PacketId = h.PacketId
	r.Header.PayloadLength = 8

	for _, x := range g.SearchRangeList {
		vb := c.getNextVarBind(x.String(), next)
		//log.Printf("out: %s", vb.Name.String())
		r.VarBindList = append(r.VarBindList, vb)
		r.Header.PayloadLength += int32(vb.WireSize())
	}
	sendMsg(&r, c)
}

type HandlerType int

const (
	GetHandlerType        = 1
	GetSubtreeHandlerType = 2
	TestSetHandlerType    = 3
)

type HandlerBundle struct {
	Oid     string
	Type    HandlerType
	Handler interface{}
}

type HandlerBundles []HandlerBundle

func (hs HandlerBundles) Len() int           { return len(hs) }
func (hs HandlerBundles) Swap(i, j int)      { hs[i], hs[j] = hs[j], hs[i] }
func (hs HandlerBundles) Less(i, j int) bool { return hs[i].Oid < hs[j].Oid }

//TODO it's probably inefficient to sort every time maybehapps this information
//     should be cached somewhere
func (c *Connection) getNextVarBind(oid string, next bool) VarBind {

	//log.Printf("[get-next-vb] oid=%s next=%v", oid, next)

	//make the array to hold the handlers that has a size equal to the sum of
	//the two handler maps
	allHandlers := make(HandlerBundles, 0,
		len(c.getSubtreeHandlers)+len(c.getHandlers))

	//if the list of handlers does not contain what we are looking for exactly
	//then the 'next' entry is actually the first entry found by the recursive
	//search algorithm
	if _, ok := c.getHandlers[oid]; !ok {
		if _, ok := c.getSubtreeHandlers[oid]; !ok {
			//next = false
		}
	}

	//bundle up the handlers in to one list and sort it according to oid
	for k, v := range c.getSubtreeHandlers {
		allHandlers = append(allHandlers,
			HandlerBundle{Oid: k, Type: GetSubtreeHandlerType, Handler: v})
	}
	for k, v := range c.getHandlers {
		allHandlers = append(allHandlers,
			HandlerBundle{Oid: k, Type: GetHandlerType, Handler: v})
	}
	sort.Sort(allHandlers)

	//return whatever var search comes up with
	return varSearch(oid, allHandlers, next)
}

// varSearch is a recursive algorithm for binding ain input oid to a variable
// instance. In the case that next is false, it binds to the first matching oid
// it finds, otherwise it binds to the following oid.
func varSearch(oid string, handlers []HandlerBundle, next bool) VarBind {
	//log.Printf("[var-search] oid=%s next=%v", oid, next)
	subtree, _ := NewSubtree(oid)
	if len(handlers) == 0 {
		return EndOfMibViewVarBind(*subtree)
	}
	h := handlers[0]
	h_subtree, _ := NewSubtree(h.Oid)
	if h.Type == GetSubtreeHandlerType {
		//truncate the target oid to the prefix length of the handler, if the
		//handler comes at or after the truncation it should be executed
		if h.Oid >= oid[:len(h.Oid)] {
			vb := h.Handler.(GetSubtreeHandler)(*subtree, next)
			//if the subtree does not have the target oid we fall through to continue
			//searching
			if vb.Type != EndOfMibViewT {
				return vb
			}
		}
	} else {
		if h.Oid >= oid {
			if next {
				next = false
			} else {
				return h.Handler.(GetHandler)(*h_subtree)
			}
		}
	}
	//recursive continuation
	return varSearch(oid, handlers[1:], next)
}

// set handling ...............................................................
func handleTestSet(c *Connection, h *Header, buf []byte) {

	var m SetMessage
	m.UnmarshalBinary(buf)

	r := Response{
		Header: Header{
			Version:       1,
			Type:          ResponsePDU,
			Flags:         h.Flags & NetworkByteOrder,
			SessionId:     c.sessionId,
			TransactionId: h.TransactionId,
			PacketId:      h.PacketId,
			PayloadLength: 8,
		},
		ResponsePayload: ResponsePayload{
			Error: int16(TestSetResourceUnavailable),
		},
	}

	hbs := make(HandlerBundles, 0, len(c.testSetHandlers))
	for name, h := range c.testSetHandlers {
		hbs = append(hbs, HandlerBundle{
			Oid:     name,
			Type:    TestSetHandlerType,
			Handler: h,
		})
	}
	sort.Sort(hbs)

	for _, v := range m.VarBindList {

		for _, h := range hbs {
			if strings.HasPrefix(v.Name.String(), h.Oid) {
				r.ResponsePayload.Error =
					int16(h.Handler.(TestSetHandler)(v, int(c.sessionId)))
			}
		}

	}

	sendMsg(&r, c)

}

func handleCommitSet(c *Connection, h *Header, buf []byte) {

	result := c.commitSetHandler(int(h.SessionId))

	r := Response{
		Header: Header{
			Version:       1,
			Type:          ResponsePDU,
			Flags:         h.Flags & NetworkByteOrder,
			SessionId:     c.sessionId,
			TransactionId: h.TransactionId,
			PacketId:      h.PacketId,
			PayloadLength: 8,
		},
		ResponsePayload: ResponsePayload{
			Error: int16(result),
		},
	}

	sendMsg(&r, c)

}

func handleCleanupSet(c *Connection, h *Header, buf []byte) {

	c.cleanupSetHandler(int(h.SessionId))

}
