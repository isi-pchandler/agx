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

type QVSTable []*agx.VarBind

func (t QVSTable) Len() int           { return len(t) }
func (t QVSTable) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t QVSTable) Less(i, j int) bool { return t[i].Name.LessThan(t[j].Name) }

var qtable QVSTable
var swptable []int

/*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 *
 * Entry Point
 *
 *~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~*/

func main() {

	qvs_subtree, _ := agx.NewSubtree(qvs)

	qtable = generateQVSTable()
	swptable = generateSWPTable()

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

		table := generateVlanTable()
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

		if len(qtable) == 0 {
			log.Printf("vlan table is empty")
			return agx.EndOfMibViewVarBind(oid)
		}

		if oid.HasPrefix(*qvs_subtree) {
			entry := findEntry(oid, next)
			if entry == nil {
				return agx.EndOfMibViewVarBind(oid)
			} else {
				return *entry
			}
		} else {
			log.Printf("[qvs]top level requested - returning first vlan entry name")
			return *qtable[0]
		}

	})

	c.OnTestSet(qvs, func(vb agx.VarBind, sessionId int) agx.TestSetResult {

		log.Printf("[test-set] oid::%s session=%d", vb.Name.String(), sessionId)

		table, vid, err := parseOid(vb.Name.String())
		if err != nil {
			log.Printf("[test-set] error parsing oid=%s", vb.Name.String())
			return agx.TestSetGenError
		}

		if table == qvs_egress_suffix {

			log.Printf("[test-set] egress vid=%d", vid)
			s, ok := vb.Data.(agx.OctetString)
			if !ok {
				log.Printf(
					"[test-set] error setting egress: varbind must be an octet string")
				return agx.TestSetWrongType
			}
			log.Printf("setting egress = %v", s)
			err = setVlans(vid, s, false)
			if err != nil {
				log.Printf("error setting egress vlans: %v", err)
				return agx.TestSetGenError
			}

		} else if table == qvs_untagged_suffix {

			log.Printf("[test-set] access vid=%d", vid)
			s, ok := vb.Data.(agx.OctetString)
			if !ok {
				log.Printf(
					"[test-set] error setting access: varbind must be an octet string")
				return agx.TestSetWrongType
			}
			log.Printf("setting access = %v", s)
			err = setVlans(vid, s, true)
			if err != nil {
				log.Printf("error setting access vlans: %v", err)
				return agx.TestSetGenError
			}

		}

		return agx.TestSetNoError

	})

	c.OnCommitSet(func(sessionId int) agx.CommitSetResult {

		log.Printf("[commit-set] session=%d", sessionId)

		return agx.CommitSetNoError

	})

	c.OnCleanupSet(func(sessionId int) {

		log.Printf("[cleanup-set] session=%d", sessionId)

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
			"[q_static] bad oid::%s type [%s : %s] %v",
			oid, oid[len(qvs):], suffix[0], err)
	}

	entry_num, err := strconv.Atoi(suffix[1])
	if err != nil {
		return -1, -1, fmt.Errorf(
			"[q_static] bad oid::%s index [%s : %s] %v",
			oid, oid[len(qvs):], suffix[0], err)
	}

	return entry_type, entry_num, nil
}

// Helpers ====================================================================

func findEntry(oid agx.Subtree, next bool) *agx.VarBind {

	//binary search for the variable we are looking for
	//it's the smallest value greater than the target oid
	i := sort.Search(
		len(qtable),
		func(i int) bool { return qtable[i].Name.GreaterThanEq(oid) },
	)

	//binary search found nothing
	if i == -1 {
		log.Printf("findEntry oid=%s next=%v not found", oid.String(), next)
		return nil
	}
	if !next {
		log.Printf("findEntry returning next=%s", qtable[i].Name.String())
		return qtable[i]
	}
	if i < len(qtable)-1 {
		log.Printf("findEntry returning next=%s", qtable[i+1].Name.String())
		return qtable[i+1]
	}

	log.Printf("findEntry oid=%s next=%v i=%d not found", oid.String(), next, i)
	return nil
}

//Genertes a table keyed by vlan number
func generateVlanTable() VlanTable {
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

func generateSWPTable() []int {

	var result []int

	links, err := netlink.LinkList()
	if err != nil {
		log.Printf("fail to get list of physical links")
		return nil
	}

	for _, l := range links {
		if strings.HasPrefix(l.Attrs().Name, "swp") {
			result = append(result, l.Attrs().Index)
		}
	}

	return result

}

//Generates the 'Vlan Static' Table
func generateQVSTable() QVSTable {
	table := make(map[string]*agx.VarBind)

	bridges, _ := netlink.GetBridgeInfo()
	vtable_length := int(math.Ceil(float64(len(bridges)) / 8))
	for bridge_index, bridge := range bridges {
		for _, vlan := range bridge.Vlans {

			//generate the name, egress and access oid tags for the current vlan
			name_tag := fmt.Sprintf("%s.%d", qvs_name, vlan.Vid)
			name_oid, _ := agx.NewSubtree(name_tag)

			egress_tag := fmt.Sprintf("%s.%d", qvs_egress, vlan.Vid)
			egress_oid, _ := agx.NewSubtree(egress_tag)

			access_tag := fmt.Sprintf("%s.%d", qvs_untagged, vlan.Vid)
			access_oid, _ := agx.NewSubtree(access_tag)

			//each vlan gets a name
			entry := &agx.VarBind{
				Type: agx.OctetStringT,
				Name: *name_oid,
				Data: *agx.NewOctetString([]byte(fmt.Sprintf("v%d", vlan.Vid))),
			}
			table[name_tag] = entry

			//ensure the vlan has access and egress tables
			ok := false
			entry, ok = table[egress_tag]
			if !ok {
				entry =
					agx.OctetStringVarBind(*egress_oid, make([]byte, vtable_length))
				table[egress_tag] = entry
			}
			entry, ok = table[access_tag]
			if !ok {
				entry =
					agx.OctetStringVarBind(*access_oid, make([]byte, vtable_length))
				table[access_tag] = entry
			}

			//set the egress and access tables for each vlan
			if vlan.Untagged {
				entry, _ = table[egress_tag]
				SetPort(bridge_index, entry.Data.(agx.OctetString).Octets[:])
			} else {
				entry, _ = table[access_tag]
				SetPort(bridge_index, entry.Data.(agx.OctetString).Octets[:])
			}
		}
	}

	//translate the unordered table created above into an ordered_table
	ordered_table := make(QVSTable, 0, len(table))
	for _, e := range table {
		ordered_table = append(ordered_table, e)
	}
	sort.Sort(ordered_table)

	for _, e := range ordered_table {
		log.Printf("==>%s = %v", e.Name.String(), e)
	}
	return ordered_table
}

func setVlans(vid int, table agx.OctetString, access bool) error {
	bridge_flags := uint(0)
	vinfo_flags := uint(0)
	if access {
		vinfo_flags |= netlink.BRIDGE_VLAN_INFO_UNTAGGED
	}

	for i := 0; i < len(swptable); i++ {
		if IsPortSet(i, table.Octets) {
			log.Printf("vlan-add i=%d access=%v", swptable[i], access)
			err := netlink.BridgeVlanAdd(
				uint(vid), swptable[i], bridge_flags, vinfo_flags)
			if err != nil {
				return err
			}
		} else {
			log.Printf("vlan-del i=%d access=%v", swptable[i], access)
			err := netlink.BridgeVlanDel(
				uint(vid), swptable[i], bridge_flags, vinfo_flags)
			if err != nil {
				return err
			}
		}
	}

	return nil
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
