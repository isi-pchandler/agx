# agx
An [AgentX](https://tools.ietf.org/html/rfc2741) library for Go

## Rationale
There are alreay a few other [AgentX](https://tools.ietf.org/html/rfc2741) libraries for Go out there. However, none of the ones I found seem to support setting variables, and most seem to be built with a relatively static devices in mind. **agx** is purposely designed to support managing highly dynamic devices. Both set and get operations are exposed through functional interfaces that allow your code to be executed when GET or SET operations come through the pipes.

## Disclaimer 
I am still pushing toward an initial release and the library is not yet fully functional.

## Basic Usage
The example below sets up an agent to manage vlans using the [Q-BRIDGE](https://tools.ietf.org/html/rfc4363) standard.
```go
package main

import "github.com/rcgoodfellow/agx"

func main() {
	id, descr := "qbridge-agent", "agent for controlling valns"
	qbridge := "1.3.6.1.2.1.17"
	
	c, err := agx.Connect(&id, &descr)
	defer c.Disconnect()
	
	c.Register(qbridge)
	defer c.Unregister(qbridge)

	c.OnGet(qbridge, func(oid agx.Subtree) agx.VarBind {

		var v agx.VarBind
		v.Type = agx.OctetStringT
		v.Name = oid
		v.Data = *agx.NewOctetString(string([]byte{0xcc, 0x33}))
		return v

	})

	//wait for connection to close
	<-c.Closed
}
```
