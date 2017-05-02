# agx
An [AgentX](https://tools.ietf.org/html/rfc2741) library for Go

## Rationale
There are alreay a few other [AgentX](https://tools.ietf.org/html/rfc2741) libraries for Go out there. However, none of the ones I found seem to support setting variables, and most seem to be built with a relatively static devices in mind. **agx** is purposely designed to support managing highly dynamic devices. Both set and get operations are exposed through a functional interfaces that allow your code to be executed when a GET or SET operations come through the pipes.

## Disclaimer 
I am still pushing toward an initial release and the library is not yet fully functional.

## Basic Usage
The example below sets up an agent to manage vlans using the [Q-BRIDGE](https://tools.ietf.org/html/rfc4363) standard.
```go
package main
import "github.com/rcgoodfellow/agx"

const (
	qbridge = "1.3.5.1.2.1.17"
	egress  = "1.3.6.1.2.1.17.7.1.4.3.1.2"
)

int main(){
	id, descr := "1.2.3.4.7", "qbridge agent"
	c, err := agx.Connect(&id, &descr)
	defer c.Disconnect()

	agent, err = c.Register(qbridge)
	defer c.Unregister(qbridge)

	agent.OnGet(egress, func(oid agx.OID) agx.PDU {
		return agx.NewOctetString(getVlanState())
	})

	agent.OnSet(egress, func(oid agx.OID, pdu agx.PDU) {
		setVlanState(pdu.Bytes())
		agent.Quit()
	})
	
	agent.Wait()
}
```
