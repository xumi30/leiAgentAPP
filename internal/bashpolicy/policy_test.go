package bashpolicy

import "testing"

func TestValidateDefaultBlacklist(t *testing.T) {
	_ = SetRules(DefaultRules())
	if err := ValidateCommand(":(){:|:&};:"); err == nil {
		t.Fatal("fork bomb expected blocked")
	}
	if err := ValidateCommand("ls"); err != nil {
		t.Fatal("simple cmd:", err)
	}
	blocked := []string{
		"chmod -R 777 /",
		"chown -R root:root /",
		"mv /* /dev/null",
		"mount /dev/sdb1 /",
		"cp /dev/null /bin/bash",
		"ln -sf /dev/null /bin/login",
		"iptables -F && iptables -X",
		"iptables -t nat -F",
		"ip6tables -F",
		"dd of=/dev/sda",
		"nft flush ruleset",
		"find / -delete",
		"find / -exec rm {}",
	}
	for _, cmd := range blocked {
		if err := ValidateCommand(cmd); err == nil {
			t.Fatalf("expected blocked: %q", cmd)
		}
	}
}
