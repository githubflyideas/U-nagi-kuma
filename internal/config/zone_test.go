package config

import "testing"

const zoneBody = "@ IN SOA node. admin.localhost. ( 1 1m 15s 5s 3 )\nweb IN HTTP https://example.com\n"

func TestZoneParseCompatibility(t *testing.T) {
	z, c, _, err := ParseZone("zone.bind", []byte(zoneBody), Options{})
	if err != nil || z != nil || c == nil || len(c.Monitors[0].Nodes) != 0 {
		t.Fatalf("legacy changed: %v %v", z, err)
	}
	text := "@ IN SOA node. admin.localhost. ( 1 1m 15s 5s 3 )\n$ZONE zone_id=12345678-1234-4234-8234-123456789abc name=office generation=1 source_node=node\nweb IN HTTP https://example.com\n"
	z, c, _, err = ParseZone("zone.bind", []byte(text), Options{})
	if err != nil || z == nil || z.Name != "office" || z.Serial != 1 || len(z.ContentHash) != 64 || c.Serial != 1 {
		t.Fatalf("zone parse: %#v %v", z, err)
	}
	for _, suffix := range []string{" role=primary", " generation=2", " unknown=x"} {
		bad := "@ IN SOA node. admin.localhost. ( 1 1m 15s 5s 3 )\n$ZONE zone_id=12345678-1234-4234-8234-123456789abc name=office generation=1" + suffix + "\n"
		if _, _, _, err := ParseZone("x", []byte(bad), Options{}); err == nil {
			t.Fatalf("accepted %s", suffix)
		}
	}
}
