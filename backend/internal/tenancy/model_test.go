package tenancy

import "testing"

func TestMembershipStatusTransitions(t *testing.T) {
	tests := []struct {
		from    MembershipStatus
		to      MembershipStatus
		allowed bool
	}{
		{from: MembershipStatusActive, to: MembershipStatusDisabled, allowed: true},
		{from: MembershipStatusActive, to: MembershipStatusRemoved, allowed: true},
		{from: MembershipStatusDisabled, to: MembershipStatusActive, allowed: true},
		{from: MembershipStatusDisabled, to: MembershipStatusRemoved, allowed: true},
		{from: MembershipStatusActive, to: MembershipStatusActive, allowed: false},
		{from: MembershipStatusDisabled, to: MembershipStatusDisabled, allowed: false},
		{from: MembershipStatusRemoved, to: MembershipStatusActive, allowed: false},
		{from: MembershipStatusRemoved, to: MembershipStatusDisabled, allowed: false},
		{from: MembershipStatusRemoved, to: MembershipStatusRemoved, allowed: false},
	}

	for _, test := range tests {
		if actual := CanTransitionMembership(test.from, test.to); actual != test.allowed {
			t.Fatalf("transition %s -> %s: got %t, want %t", test.from, test.to, actual, test.allowed)
		}
	}
}
