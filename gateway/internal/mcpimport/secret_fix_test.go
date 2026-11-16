package mcpimport

import "testing"

// FIX-50 half one: -k and -p are the common short spellings for an API key and
// a password on an MCP command line. They used to fall through as safe args,
// so the value was stored.
func TestShortSecretFlagsKAndP(t *testing.T) {
	cases := []string{
		"-k",
		"-k=abc",
		"-kabc",
		"-p",
		"-p=hunter2",
		"-p hunter2",
	}
	for _, c := range cases {
		if !argSecret(c) {
			t.Fatalf("%q must be held as a secret", c)
		}
	}
}

// A short flag that is not a credential stays usable, otherwise -n and friends
// become unusable and the importer over-holds every command.
func TestShortFlagsThatStaySafe(t *testing.T) {
	for _, c := range []string{"-n", "-v", "-x=1"} {
		if argSecret(c) {
			t.Fatalf("%q must stay usable", c)
		}
	}
}

// FIX-50 half two: a credential can arrive as an HTTP header, not only as a
// switch. The header name, not the whole line, decides. These were stored
// before because headerName did not exist and "x-api-key" has no leading dash.
func TestHeaderNamesHeldAsSecrets(t *testing.T) {
	// The colon is what marks the field as a header line. A bare dashed token
	// is not a header: an api_key value can look like one, and holding those
	// would block ordinary rows.
	cases := []string{
		"X-Api-Key: abc",
		"x-api-key:",
		"X-Auth-Token: abc",
		"x-auth-token:abc",
		"Authorization-Key: abc",
		"PRIVATE-TOKEN: abc",
		"Private-Token:abc",
		"X-Access-Token:abc",
		"X-Client-Secret: abc",
		"X-Password: abc",
		"X-Secret: abc",
		"X-Credential: abc",
		"X-Bearer: abc",
		"X-Auth: abc",
		"X-Pat: abc",
	}
	for _, c := range cases {
		if !argSecret(c) {
			t.Fatalf("%q must be held as a secret", c)
		}
	}
}

// "--header=X-Api-Key: abc" hides the credential after an '=' under a switch
// that is not itself named like one. The value side has to be classified.
func TestHeaderThroughSwitchValue(t *testing.T) {
	cases := []string{
		"--header=X-Api-Key: abc",
		"--header=X-Auth-Token: abc",
		"-H=X-Api-Key: abc",
	}
	for _, c := range cases {
		if !argSecret(c) {
			t.Fatalf("%q must be held as a secret", c)
		}
	}
}

// A plain header that carries no credential must not be held, or every MCP
// server that sets Accept or Content-Type is blocked.
func TestHarmlessHeadersStaySafe(t *testing.T) {
	cases := []string{
		"Accept: application/json",
		"Content-Type: application/json",
		"User-Agent: goso-import",
		"--header=Accept: application/json",
		"--header=Content-Type: application/json",
	}
	for _, c := range cases {
		if argSecret(c) {
			t.Fatalf("%q must stay usable", c)
		}
	}
}

// A long path or a host:port is an ordinary argument, not a credential.
// These parked eight master connectors on goso-full (audit N8, 16/11).
func TestPathsAndAddressesAreNotKeys(t *testing.T) {
	for _, c := range []string{
		"host.docker.internal:18891",
		"/app/mcp/dembrandt-mcp-shim.js",
		"./node_modules/.bin/some-mcp-server",
		"mcp-server-filesystem-readonly",
	} {
		if argSecret(c) {
			t.Fatalf("%q must stay usable", c)
		}
	}
	for _, c := range []string{
		// Built by concatenation so the secret scanners do not flag a fake.
		"sk" + "-proj-4f9aB2cD7eF1gH3iJ5kL",
		"ghp" + "_1234567890abcdefghijABCDEFGHIJ",
		"a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
	} {
		if !argSecret(c) {
			t.Fatalf("%q must be held as a secret", c)
		}
	}
}
