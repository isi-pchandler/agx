package main

import (
	"github.com/rcgoodfellow/agx"
	"github.com/rcgoodfellow/netlink"
	"log"
)

/*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 *
 * MIB Objects
 *
 *~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~*/

// top level objects
const (
	qbridge  = "1.3.6.1.2.1.17"
	q_base   = qbridge + ".7.1.1"
	q_tp     = qbridge + ".7.1.2"
	q_static = qbridge + ".7.1.3"
	q_vlan   = qbridge + ".7.1.4"
)

// base
const (
	qb_version        = q_base + ".1.0"
	qb_maxvlanid      = q_base + ".2.0"
	qb_supportedvlans = q_base + ".3.0"
	qb_numvlans       = q_base + ".4.0"
	qb_gvrp           = q_base + ".5.0"
)

/*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 *
 * System Constants
 *
 *~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~*/

const (
	vlan_version        = 1
	max_vlanid          = 4094
	max_supported_vlans = 4094
	gvrp_status         = 2
)

/*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 *
 * Data Structures
 *
 *~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~*/

type VlanTableEntry struct {
	Vlan       *netlink.VlanInfo
	Interfaces []int
}

type VlanTable map[int]*VlanTableEntry

/*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 *
 * Entry Point
 *
 *~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~*/

func main() {

	id, descr := "1.2.3.4.7", "qbridge-agent"
	c, err := agx.Connect(&id, &descr)
	if err != nil {
		log.Fatalf("connection failed %v", err)
	}
	defer c.Disconnect()

	err = c.Register(qbridge)
	if err != nil {
		log.Fatalf("agent registration failed %v", err)
	}
	defer func() {
		err = c.Unregister(qbridge)
		if err != nil {
			log.Fatalf("agent registration failed %v", err)
		}
	}()

	c.OnGet(qb_version, func(oid agx.Subtree) agx.VarBind {

		log.Printf("[qbridge][get] version=%d", vlan_version)
		return agx.IntegerVarBind(oid, vlan_version)

	})

	c.OnGet(qb_maxvlanid, func(oid agx.Subtree) agx.VarBind {

		log.Printf("[qbridge][get] maxvlanid=%d", max_vlanid)
		return agx.IntegerVarBind(oid, max_vlanid)

	})

	c.OnGet(qb_supportedvlans, func(oid agx.Subtree) agx.VarBind {

		log.Printf("[qbridge][get] supportedvlans=%d", max_supported_vlans)
		return agx.Gauge32VarBind(oid, max_supported_vlans)

	})

	c.OnGet(qb_numvlans, func(oid agx.Subtree) agx.VarBind {

		table := fetchVlanTable()
		numvlans := uint32(len(table))
		log.Printf("[qbridge][get] numvlans=%d", numvlans)
		return agx.Gauge32VarBind(oid, numvlans)

	})

	c.OnGet(qb_gvrp, func(oid agx.Subtree) agx.VarBind {

		log.Printf("[qbridge][get] gvpr=%d", gvrp_status)
		return agx.IntegerVarBind(oid, gvrp_status)

	})

	c.OnGetSubtree(q_static, func(oid agx.Subtree) agx.VarBind {
		return agx.EndOfMibViewVarBind(oid)
	})

	//wait for connection to close
	log.Printf("waiting for close event")
	<-c.Closed
	log.Printf("test finished")

}

func fetchVlanTable() VlanTable {

	bridges, _ := netlink.GetBridgeInfo()
	table := make(VlanTable)
	for _, bridge := range bridges {
		for _, vlan := range bridge.Vlans {
			vid := int(vlan.Vid)
			entry, ok := table[vid]
			if ok {
				entry.Interfaces = append(
					entry.Interfaces, bridge.Index)
			} else {
				table[vid] = &VlanTableEntry{
					Vlan:       vlan,
					Interfaces: []int{bridge.Index},
				}
			}
		}
	}
	return table
}
