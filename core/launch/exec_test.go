package launch

import (
	"reflect"
	"testing"
)

func TestExecArgs(t *testing.T) {
	cases := []struct {
		exec  string
		files []string
		want  []string
	}{
		{"firefox %u", nil, []string{"firefox"}},
		{"firefox %u", []string{"a b", "c"}, []string{"firefox", "a b"}},
		{"gimp %F", []string{"a", "b"}, []string{"gimp", "a", "b"}},
		{`"/opt/My App/run" --name=%c %i --file=%f 100%%`, []string{"x"}, []string{"/opt/My App/run", "--name=Tool", "--icon", "tool", "--file=x", "100%"}},
		{`sh -c "echo \"\$HOME\" \\\\" %k`, nil, []string{"sh", "-c", `echo "$HOME" \\`, "/a.desktop"}},
		{"app %d %D %n %N %v %m", nil, []string{"app", "", "", "", "", "", ""}},
	}
	for _, c := range cases {
		got, err := ExecArgs(c.exec, c.files, "Tool", "tool", "/a.desktop")
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q %v, want %q", c.exec, got, err, c.want)
		}
	}
	if _, err := ExecArgs(`app "open`, nil, "", "", ""); err == nil {
		t.Error("unterminated quote")
	}
}

func TestFileURI(t *testing.T) {
	if got := FileURI("/home/me/A, b's.txt"); got != "file:///home/me/A%2C%20b%27s.txt" {
		t.Fatal(got)
	}
}
