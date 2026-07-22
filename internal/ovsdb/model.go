package ovsdb

import (
	"encoding/json"
	"fmt"
)

// Bridge, Port and Interface are the flattened view of the OVSDB tables we care
// about. In OVSDB these are three separate tables joined by UUID references
// (Bridge.ports -> Port, Port.interfaces -> Interface); Bridges() does that join
// so callers get a ready-to-print tree.
type Bridge struct {
	Name         string
	DatapathType string
	Ports        []Port
}

type Port struct {
	Name       string
	Interfaces []Interface
}

type Interface struct {
	Name       string
	Type       string
	AdminState string
	LinkState  string
	Statistics map[string]int64
}

// selectRows runs a single OVSDB "select" transaction and returns the matched
// rows. columns must include "_uuid" when the caller needs to join tables.
func (c *Client) selectRows(table string, columns []string) (map[string]map[string]json.RawMessage, []string, error) {
	op := map[string]interface{}{
		"op":      "select",
		"table":   table,
		"where":   []interface{}{},
		"columns": columns,
	}
	res, err := c.Call("transact", "Open_vSwitch", op)
	if err != nil {
		return nil, nil, err
	}
	var results []struct {
		Rows  []map[string]json.RawMessage `json:"rows"`
		Error string                       `json:"error"`
	}
	if err := json.Unmarshal(res, &results); err != nil {
		return nil, nil, fmt.Errorf("decoding %s rows: %w", table, err)
	}
	if len(results) == 0 {
		return map[string]map[string]json.RawMessage{}, nil, nil
	}
	if results[0].Error != "" {
		return nil, nil, fmt.Errorf("ovsdb select %s: %s", table, results[0].Error)
	}
	byUUID := map[string]map[string]json.RawMessage{}
	var order []string
	for _, row := range results[0].Rows {
		uuid, err := decodeUUID(row["_uuid"])
		if err != nil {
			continue
		}
		byUUID[uuid] = row
		order = append(order, uuid)
	}
	return byUUID, order, nil
}

// Bridges returns every bridge with its ports and interfaces resolved.
func (c *Client) Bridges() ([]Bridge, error) {
	ifaces, _, err := c.selectRows("Interface",
		[]string{"_uuid", "name", "type", "admin_state", "link_state", "statistics"})
	if err != nil {
		return nil, err
	}
	ports, _, err := c.selectRows("Port", []string{"_uuid", "name", "interfaces"})
	if err != nil {
		return nil, err
	}
	bridges, order, err := c.selectRows("Bridge", []string{"_uuid", "name", "datapath_type", "ports"})
	if err != nil {
		return nil, err
	}

	var out []Bridge
	for _, buuid := range order {
		brow := bridges[buuid]
		b := Bridge{
			Name:         rawToString(brow["name"]),
			DatapathType: rawToString(brow["datapath_type"]),
		}
		for _, puuid := range decodeUUIDSet(brow["ports"]) {
			prow, ok := ports[puuid]
			if !ok {
				continue
			}
			p := Port{Name: rawToString(prow["name"])}
			for _, iuuid := range decodeUUIDSet(prow["interfaces"]) {
				irow, ok := ifaces[iuuid]
				if !ok {
					continue
				}
				p.Interfaces = append(p.Interfaces, Interface{
					Name:       rawToString(irow["name"]),
					Type:       rawToString(irow["type"]),
					AdminState: rawToString(irow["admin_state"]),
					LinkState:  rawToString(irow["link_state"]),
					Statistics: decodeInt64Map(irow["statistics"]),
				})
			}
			b.Ports = append(b.Ports, p)
		}
		out = append(out, b)
	}
	return out, nil
}

// HWOffloadEnabled reports whether hardware offload is turned on cluster-wide,
// read from Open_vSwitch.other_config["hw-offload"].
func (c *Client) HWOffloadEnabled() (bool, error) {
	rows, _, err := c.selectRows("Open_vSwitch", []string{"_uuid", "other_config"})
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if decodeStringMap(row["other_config"])["hw-offload"] == "true" {
			return true, nil
		}
	}
	return false, nil
}
