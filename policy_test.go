package metagrab

import (
	"net/url"
	"testing"
)

func TestDenyPrivateIPs_Loopback(t *testing.T) {
	policy := DenyPrivateIPs()
	u, _ := url.Parse("http://localhost/test")
	err := policy(u)
	if err == nil {
		t.Fatal("expected loopback to be denied")
	}
	t.Logf("correctly denied: %v", err)
}

func TestDenyPrivateIPs_PublicIP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DNS lookup in short mode")
	}
	policy := DenyPrivateIPs()
	// google.com resolves to public IPs
	u, _ := url.Parse("https://google.com")
	err := policy(u)
	if err != nil {
		t.Fatalf("expected public IP to be allowed, got: %v", err)
	}
}

func TestDenyPrivateIPs_UnresolvableHost(t *testing.T) {
	policy := DenyPrivateIPs()
	u, _ := url.Parse("http://this-host-does-not-exist-xyz123.invalid/test")
	err := policy(u)
	if err == nil {
		t.Fatal("expected unresolvable host to return error")
	}
	t.Logf("correctly errored: %v", err)
}

func TestDenyPrivateIPs_PrivateRanges(t *testing.T) {
	policy := DenyPrivateIPs()
	// 127.0.0.1 is guaranteed to resolve for localhost
	u, _ := url.Parse("http://127.0.0.1/secret")
	err := policy(u)
	if err == nil {
		t.Fatal("expected 127.0.0.1 to be denied")
	}
}
