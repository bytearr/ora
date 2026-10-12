package discover

import "testing"

func TestParseStartApps(t *testing.T) {
	one, err := parseStartApps([]byte(`{"Name":"Calculator","AppID":"Calc!App"}`))
	if err != nil || len(one) != 1 || one[0].AppID != "Calc!App" {
		t.Errorf("single: %v %v", one, err)
	}
	many, err := parseStartApps([]byte("\ufeff[{\"Name\":\"A\",\"AppID\":\"a!b\"},{\"Name\":\"B\",\"AppID\":\"Chrome\"}]"))
	if err != nil || len(many) != 2 {
		t.Errorf("array: %v %v", many, err)
	}
}
