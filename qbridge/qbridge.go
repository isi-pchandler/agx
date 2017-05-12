package main

import (
	"fmt"
	"github.com/rcgoodfellow/agx"
	"github.com/rcgoodfellow/netlink"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
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

// vlan static
const (
	qvs_name_suffix             = 1
	qvs_egress_suffix           = 2
	qvs_forbidden_egress_suffix = 3
	qvs_untagged_suffix         = 4
	qvs_status_suffix           = 5
)
const (
	qvs                  = q_vlan + ".3.1"
	qvs_name             = qvs + ".1"
	qvs_egress           = qvs + ".2"
	qvs_forbidden_egress = qvs + ".3"
	qvs_untagged         = qvs + ".4"
	qvs_status           = qvs + ".5"
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

/*
type QVSTableEntry struct {
	VID    int
	Egress []int
	Access []int
}
*/
type QVSTableEntry struct {
	OID   string
	Type  int
	Value interface{}
}
type QVSTable []*QVSTableEntry

var vtable QVSTable

func (t QVSTable) Len() int           { return len(t) }
func (t QVSTable) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t QVSTable) Less(i, j int) bool { return t[i].OID < t[j].OID }

/*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 *
 * Entry Point
 *
 *~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~*/

func main() {

	vtable := generateQVSTable()

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

	//Vlan Base +++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++

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

	//Vlan Table ++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++

	c.OnGetSubtree(qvs, func(oid agx.Subtree, next bool) agx.VarBind {

		if len(vtable) == 0 {
			log.Printf("vlan table is empty")
			return agx.EndOfMibViewVarBind(oid)
		}

		name := oid.String()
		log.Printf("[qvs][get-subtree] oid=%s next=%v", name, next)

		if strings.HasPrefix(name, qvs) {

			entry_type, entry_num, err := parseOid(name)
			entry := findEntry(name, next)
			if err != nil {
				log.Print(err)
				return agx.NoSuchObjectVarBind(oid)
			}

			if entry == nil {
				log.Printf("[q_static] no vlan table for %d next=%v", entry_num, next)
				return agx.EndOfMibViewVarBind(oid)
			}

			if entry_type == qvs_name_suffix {
				return agx.OctetStringVarBind(oid, fmt.Sprintf("vlan-%d", entry.Value.(int)))
			}

		} else {
			log.Printf("[qvs]top level requested - returning first vlan entry name")
			entry_name := fmt.Sprintf("", string(vtable[0].Value.(agx.OctetString).Octets))
			entry_oid, _ := agx.NewSubtree(vtable[0].OID)
			return agx.OctetStringVarBind(*entry_oid, entry_name)
		}

		return agx.EndOfMibViewVarBind(oid)

	})

	c.OnTestSet(q_static, func(vb agx.VarBind) agx.TestSetResult {
		log.Printf("[q_static][test-set] oid::%s", vb.Name.String())
		return agx.TestSetNoError
	})

	//wait for connection to close
	log.Printf("waiting for close event")
	<-c.Closed
	log.Printf("test finished")
}

func parseOid(oid string) (int, int, error) {
	suffix := strings.Split(oid[len(qvs)+1:], ".")

	entry_type, err := strconv.Atoi(suffix[0])
	if err != nil {
		return -1, -1, fmt.Errorf(
			"[q_static] bad oid::%s type [%s : %s] %v", oid, oid[len(qvs):], suffix[0], err)
	}

	entry_num, err := strconv.Atoi(suffix[1])
	if err != nil {
		return -1, -1, fmt.Errorf(
			"[q_static] bad oid::%s index [%s : %s] %v", oid, oid[len(qvs):], suffix[0], err)
	}

	return entry_type, entry_num, nil
}

// Helpers ====================================================================

func findEntry(oid string, next bool) *QVSTableEntry {
	idx :=
		sort.Search(len(vtable), func(i int) bool { return vtable[i].OID >= oid })
	if idx >= 0 {
		if next {
			if idx < len(vtable)-1 {
				return vtable[idx+1]
			} else {
				return nil
			}
		} else {
			return vtable[idx]
		}
	} else {
		return nil
	}
}

//XXX
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

func generateQVSTable() QVSTable {
	table := make(map[string]*QVSTableEntry)

	bridges, _ := netlink.GetBridgeInfo()
	for bridge_index, bridge := range bridges {
		for _, vlan := range bridge.Vlans {

			name_oid := fmt.Sprintf("%s.%d", qvs_name, vlan.Vid)
			egress_oid := fmt.Sprintf("%s.%d", qvs_egress, vlan.Vid)
			access_oid := fmt.Sprintf("%s.%d", qvs_untagged, vlan.Vid)

			entry := &QVSTableEntry{}

			entry.OID = name_oid
			entry.Value = *agx.NewOctetString(fmt.Sprintf("v%d", vlan.Vid))
			entry.Type = agx.OctetStringT
			table[name_oid] = entry

			if vlan.Untagged {
				entry, ok := table[egress_oid]
				if !ok {
					entry := &QVSTableEntry{}
					entry.OID = egress_oid
					length := int(math.Ceil(float64(len(bridges)) / 8))
					value := agx.OctetString{
						OctetStringLength: int32(length),
						Octets:            make([]byte, length),
					}
					SetPort(bridge_index, value.Octets[:])
					entry.Value = value
					entry.Type = agx.OctetStringT
					table[egress_oid] = entry
				} else {
					SetPort(bridge_index, entry.Value.(agx.OctetString).Octets[:])
				}
			} else {
				entry, ok := table[access_oid]
				if !ok {
					entry := &QVSTableEntry{}
					entry.OID = egress_oid
					length := int(math.Ceil(float64(len(bridges)) / 8))
					value := agx.OctetString{
						OctetStringLength: int32(length),
						Octets:            make([]byte, length),
					}
					SetPort(bridge_index, value.Octets[:])
					entry.Value = value
					entry.Type = agx.OctetStringT
					table[access_oid] = entry
				} else {
					SetPort(bridge_index, entry.Value.(agx.OctetString).Octets[:])
				}
			}
		}
	}

	ordered_table := make(QVSTable, 0, len(table))
	for _, e := range table {
		ordered_table = append(ordered_table, e)
	}

	sort.Sort(ordered_table)
	return ordered_table
}

// IsPortSet returns whether or not the port at index i is set within the
// object ports which is an snmp style portlist data structure. For the
// details of this structure see RFC 2674 in the Textual Conventions section.
func IsPortSet(i int, ports []byte) bool {

	bits := ports[i/8]
	isSet := bits&(1<<uint(7-(i%8))) > 0
	return isSet

}

// SetPort sets the port at index i in the object ports which is an snmp
// style portlist data structure. For the details of this structure see RFC
// 2674 in the Textual Conventions section.
func SetPort(i int, ports []byte) {

	bits := &ports[i/8]
	bit := 7 - (i % 8)
	*bits |= (1 << uint(bit))

}

// UnsetPort clears the port at index i in the object ports which is an snmp
// style portlist data structure. For the details of this structure see RFC
// 2674 in the Textual Conventions section.
func UnsetPort(i int, ports []byte) {

	bits := &ports[i/8]
	bit := 7 - (i % 8)
	*bits &= ^(1 << uint(bit))

}
