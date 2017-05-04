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
)

/*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 * Connections
 *----------------------------------------------------------------------------*/
type Connection struct {
	//private members
	conn          net.Conn
	sessionId     int32
	registrations []string
	closed        bool
	getHandlers   map[string]GetHandler
	setHandlers   map[string]SetHandler

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
	c.setHandlers = make(map[string]SetHandler)

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
type SetHandler func(oid Subtree, pdu VarBind) VarBind

func (c *Connection) OnGet(oid string, f GetHandler) {
	c.getHandlers[oid] = f
}

func (c *Connection) OnSet(oid string, f SetHandler) {
	c.setHandlers[oid] = f
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
		default:
			log.Printf("[roogMH] unknown message")
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

func handleGet(c *Connection, h *Header, buf []byte) {
	g := &GetMessage{}
	_, err := g.UnmarshalBinary(buf)
	if err != nil {
		log.Printf("[get] error unmarshalling GetPDU %v\n", err)
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
		oid := x.String()
		if x.Prefix != 0 {
			oid = fmt.Sprintf("1.3.6.1.%d.%s", x.Prefix, oid)
		}
		handler, ok := c.getHandlers[oid]
		if !ok {
			log.Printf("[get] no handler for %s", oid)
			vb := NoSuchObjectVarBind(x)
			r.VarBindList = append(r.VarBindList, vb)
			r.Header.PayloadLength += int32(vb.WireSize())
		} else {
			log.Printf("[get] passing along %s", oid)
			vb := handler(x)
			r.VarBindList = append(r.VarBindList, vb)
			r.Header.PayloadLength += int32(vb.WireSize())
		}
	}

	sendMsg(&r, c)
}

func handleGetNext(c *Connection, h *Header, buf []byte) {
	log.Printf("[getnext]")
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
		oid := x.String()
		if x.Prefix != 0 {
			oid = fmt.Sprintf("1.3.6.1.%d.%s", x.Prefix, oid)
		}

		nextkey, handler, ok := c.getNextHandler(oid)
		if !ok {
			log.Printf("[get] no handler for %s", oid)
			vb := EndOfMibViewVarBind(x)
			r.VarBindList = append(r.VarBindList, vb)
			r.Header.PayloadLength += int32(vb.WireSize())
		} else {
			nextoid, err := NewSubtree(nextkey)
			if err != nil {
				log.Printf("error reading nextoid %s", nextkey)
				continue
			}
			log.Printf("[getnext] passing along %s", oid)
			vb := handler(*nextoid)
			r.VarBindList = append(r.VarBindList, vb)
			r.Header.PayloadLength += int32(vb.WireSize())
		}
	}
	sendMsg(&r, c)
}

//TODO it's probably inefficient to sort every time maybehapps this information
//     should be cached somewhere
func (c *Connection) getNextHandler(oid string) (string, GetHandler, bool) {
	keys := make([]string, 0, len(c.getHandlers))

	//if the starting key is not here add it so we can use it as a point of
	//reference in the sorted keys
	_, ok := c.getHandlers[oid]
	if !ok {
		keys = append(keys, oid)
	}

	//create a sorted list of oid keys
	for key, _ := range c.getHandlers {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	//find where the starting key is in the sorted set and return the one after
	idx := sort.SearchStrings(keys, oid)
	if idx >= len(keys)-1 {
		return "", nil, false
	} else {
		nextkey := keys[idx+1]
		return nextkey, c.getHandlers[nextkey], true
	}
}
