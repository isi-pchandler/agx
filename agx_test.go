package agx_test

import (
	"github.com/rcgoodfellow/agx"
	"log"
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
	defer c.Disconnect()

	err = c.Register(qbridge)
	if err != nil {
		t.Fatalf("agent registration failed %v", err)
	}
	defer func() {
		err = c.Unregister(qbridge)
		if err != nil {
			t.Fatalf("agent registration failed %v", err)
		}
	}()

	c.OnGet(qbridge, func(oid agx.Subtree) agx.VarBind {

		log.Println("[qbridge] handling request")

		var v agx.VarBind
		v.Type = agx.OctetStringT
		v.Name = oid
		v.Data = *agx.NewOctetString(string([]byte{0xcc, 0x33}))

		return v

	})

	//wait for connection to close
	log.Printf("waiting for close event")
	<-c.Closed
	log.Printf("test finished")

}
