package episodicimport

import (
	"strings"
	"testing"
)

// A GoClaw export can carry a customer-map row whose user_id is empty. That
// row used to abort the whole load, which is the FIX-53 half of FIX-53-50.
// The row is now skipped and every other row still loads.
func TestLoadCustomersTSVSkipsEmptyUser(t *testing.T) {
	file := "zalo-personal\tsender-minh-an-01\tuser-minh-an-01\tzalo-personal:sender-minh-an-01\n" +
		"zalo-personal\tsender-orphan\t\tzalo-personal:sender-orphan\n" +
		"feishu\tou-minh-an\tuser-minh-an-02\tfeishu:ou-minh-an\n"
	got, err := LoadCustomers(strings.NewReader(file))
	if err != nil {
		t.Fatalf("empty user_id must not abort the load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 kept rows, got %d", len(got))
	}
	if got["user-minh-an-01"] != "zalo-personal:sender-minh-an-01" {
		t.Fatalf("row 1 %q", got["user-minh-an-01"])
	}
	if got["user-minh-an-02"] != "feishu:ou-minh-an" {
		t.Fatalf("row 2 %q", got["user-minh-an-02"])
	}
	if _, ok := got[""]; ok {
		t.Fatal("empty user_id was stored")
	}
}

// One user reachable on two channels used to be fatal. The first row now wins,
// which is the rule the contacts importer (SPEC 172) and kgimport already use.
func TestLoadCustomersTSVFirstKeyWins(t *testing.T) {
	file := "zalo-personal\tsender-a\tuser-minh-an-01\tzalo-personal:sender-a\n" +
		"feishu\tou-b\tuser-minh-an-01\tfeishu:ou-b\n"
	got, err := LoadCustomers(strings.NewReader(file))
	if err != nil {
		t.Fatalf("two channels for one user must not abort: %v", err)
	}
	if got["user-minh-an-01"] != "zalo-personal:sender-a" {
		t.Fatalf("first row must win, got %q", got["user-minh-an-01"])
	}
}

// The JSON path has to agree with the TSV path, otherwise the same export read
// as JSONL still fails on the two rows the TSV path now tolerates.
func TestLoadCustomersJSONSkipsEmptyAndKeepsFirst(t *testing.T) {
	doc := strings.Join([]string{
		`{"customers":[`,
		`{"user_id":"user-minh-an-01","channel":"zalo-personal","sender_id":"sender-a"},`,
		`{"user_id":"","channel":"zalo-personal","sender_id":"sender-orphan"},`,
		`{"user_id":"user-minh-an-01","channel":"feishu","sender_id":"ou-b"}`,
		`]}`,
	}, "")
	got, err := LoadCustomers(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("json path must tolerate the same rows: %v", err)
	}
	if got["user-minh-an-01"] != "zalo-personal:sender-a" {
		t.Fatalf("json first row must win, got %q", got["user-minh-an-01"])
	}
	if _, ok := got[""]; ok {
		t.Fatal("empty user_id was stored from json")
	}
}

// A file that is not four columns is still a structural error: the row cannot
// be located in the format at all, so the load stops instead of guessing.
func TestLoadCustomersTSVWrongColumnCountStillFatal(t *testing.T) {
	if _, err := LoadCustomers(strings.NewReader("zalo-personal\tsender\tuser\n")); err == nil {
		t.Fatal("three columns must stay fatal")
	}
}
