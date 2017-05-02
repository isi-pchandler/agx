// Package agx provdes an AgentX API compliant with RFC 2741.
package agx

// This file contains the agx API
// ~~~
// Copyright Ryan Goodfellow 2017 - All Rights Reserved
// GPLv3

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"time"
)

var agents map[string]*Agent

/*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 * Connections
 *----------------------------------------------------------------------------*/
type Connection struct {
	conn      net.Conn
	sessionId int32
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
	c.conn.SetDeadline(time.Now().Add(ConnectionTimeout * time.Second))

	//try to open a new AgentX session with the master
	m, err := NewOpenMessage(id, descr)
	if err != nil {
		return nil, fmt.Errorf("error creating open message: %v", err)
	}
	hdr, buf, err := sendrecvMsg(m, c)

	//grab the response payload, extract and save the sessionId
	p := &AgentXResponsePayload{}
	_, err = p.UnmarshalBinary(buf[HeaderSize:])
	if err != nil {
		log.Printf("error reading open response playload: %v", err)
		return nil, err
	}
	c.sessionId = hdr.SessionId

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
	buf, err := msg.MarshalBinary()
	if err != nil {
		log.Printf("error marshalling close message: %v", err)
		c.conn.Close()
		return
	}
	_, buf, err = sendrecvMsg(msg, c)
	if err != nil {
		log.Printf("error closing connection")
	}

	//grab the response payload and check for errors
	p := &AgentXResponsePayload{}
	_, err = p.UnmarshalBinary(buf[HeaderSize:])
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
}

func (c *Connection) Register(oid string) (*Agent, error) {
	return c.doRegister(oid, false)
}

func (c *Connection) Unregister(oid string) (*Agent, error) {
	return c.doRegister(oid, true)
}

func (c *Connection) doRegister(oid string, unregister bool) (*Agent, error) {

	a := &Agent{}
	if agents == nil {
		agents = make(map[string]*Agent)
	}
	agents[oid] = a

	var m *RegisterMessage
	var err error
	if unregister {
		m, err = NewUnregisterMessage(oid, nil, nil)
	} else {
		m, err = NewRegisterMessage(oid, nil, nil)
	}

	m.Header.SessionId = c.sessionId
	if err != nil {
		return nil, fmt.Errorf("failed creating registration message %v", err)
	}

	_, buf, err := sendrecvMsg(m, c)

	//grab the response payload and check for errors
	p := &AgentXResponsePayload{}
	_, err = p.UnmarshalBinary(buf[HeaderSize:])
	if err != nil {
		c.conn.Close()
		return nil, fmt.Errorf("error reading close response playload: %v", err)
	}
	if p.Error != 0 {
		log.Printf("Master agent reporeted error on close %d", p.Error)
	}

	if unregister {
		log.Printf("unregistered %s with master", oid)
	} else {
		log.Printf("registered %s with master", oid)
	}

	return a, nil
}

/*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 * Agents
 *----------------------------------------------------------------------------*/
type Agent struct {
}

func (a *Agent) OnGet(oid string, f func(oid OID) PDU) {
	panic("not implemented")
}

func (a *Agent) OnSet(oid string, f func(oid OID, pdu PDU)) {
	panic("not implemented")
}

// helper functions ===========================================================

func sendMsg(m AgentXMessage, c *Connection) error {
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

func recvMsg(c *Connection) (*AgentXHeader, []byte, error) {
	buf := make([]byte, 1024)
	n, err := c.conn.Read(buf)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting message response: %v", err)
	}

	hdr := &AgentXHeader{}
	_, err = hdr.UnmarshalBinary(buf[:n])
	if err != nil {
		log.Printf("failure reading response header: %v", err)
		return nil, nil, fmt.Errorf("failure reading response header: %v", err)
	}
	return hdr, buf, nil
}

func sendrecvMsg(m AgentXMessage, c *Connection) (*AgentXHeader, []byte, error) {
	err := sendMsg(m, c)
	if err != nil {
		return nil, nil, err
	}

	return recvMsg(c)
}

func rootMessageHandler(r io.Reader) {
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		log.Printf("[rootMH] failure reading incommig message: %v", err)
	}

	hdr := &AgentXHeader{}
	_, err = hdr.UnmarshalBinary(buf)
	if err != nil {
		log.Printf("[rootMH] failure reading agentx header: %v", err)
	}

	log.Printf("[roogMH] message:\n %#v", hdr)
}
