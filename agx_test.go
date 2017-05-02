package agx_test

import (
	"github.com/rcgoodfellow/agx"
	"testing"
)

const (
	qbridge = "1.3.5.1.2.1.17"
	egress  = "1.3.6.1.2.1.17.7.1.4.3.1.2"
	access  = "1.3.6.1.2.1.17.7.1.4.3.1.4"
)

func TestConnect(t *testing.T) {

	id, descr := "1.2.3.4.7", "muffin man"
	c, err := agx.Connect(&id, &descr)
	if err != nil {
		t.Fatalf("connection failed %v", err)
	}

	_, err = c.Register(qbridge)
	if err != nil {
		t.Fatalf("agent registration failed failed %v", err)
	}

	_, err = c.Unregister(qbridge)
	if err != nil {
		t.Fatalf("agent registration failed failed %v", err)
	}

	c.Disconnect()

	/*
		TODO
			gc := make(chan agx.PDU)
			sc := make(chan agx.PDU)
			gi := make(chan agx.OID)
			si := make(chan agx.OID)

			agent.OnGet(egress, func(oid agx.OID) agx.PDU {
				pdu := agx.NewOctetString([]byte{0xcc, 0x33})
				gc <- pdu
				gi <- oid
				return pdu
			})

			agent.OnSet(egress, func(oid agx.OID, pdu agx.PDU) {
				sc <- pdu
				si <- oid
			})

			got, set := <-gc, <-sc
			gid, sid := <-gi, <-si

			t.Log("gid: %s", gid.String())
			t.Log("sid: %s", sid.String())

			if got != agx.NewOctetString([]byte{0xcc, 0x33}) {
				t.Error("unexpected get response")
			}

			if set != agx.NewOctetString([]byte{0x33, 0xcc}) {
				t.Error("unexpected set response")
			}
	*/

}
