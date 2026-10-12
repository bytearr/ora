package discover

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"

	"ora/core/config"
)

func TestIsPackagedAUMID(t *testing.T) {
	yes := []string{"Microsoft.WindowsCalculator_8wekyb3d8bbwe!App", "Spotify.Spotify_zpdnekdrzrea0!Spotify"}
	no := []string{"", "Chrome", "com.squirrel.Discord.Discord", `{6D809377-6AF0-444B-8957-A3773F02200E}\Foo\foo.exe`, "!App", "Pkg!"}
	for _, s := range yes {
		if !IsPackagedAUMID(s) {
			t.Errorf("want packaged: %q", s)
		}
	}
	for _, s := range no {
		if IsPackagedAUMID(s) {
			t.Errorf("want not packaged: %q", s)
		}
	}
}

func TestParseDisplayIcon(t *testing.T) {
	cases := map[string]string{
		`"C:\Program Files\App\app.exe",0`:  `C:\Program Files\App\app.exe`,
		`C:\Program Files\App\app.exe,0`:    `C:\Program Files\App\app.exe`,
		`C:\Program Files\App\app.exe,-101`: `C:\Program Files\App\app.exe`,
		`C:\App\app.exe`:                    `C:\App\app.exe`,
		`"C:\App\app.exe"`:                  `C:\App\app.exe`,
		`C:\a,b\app.exe`:                    `C:\a,b\app.exe`,
	}
	for in, want := range cases {
		if got := ParseDisplayIcon(in); got != want {
			t.Errorf("ParseDisplayIcon(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEverythingSearch(t *testing.T) {
	ev := config.Everything{
		Extensions:        []string{"exe", ".ahk"},
		ExcludeRegex:      config.DefaultExcludeRegex,
		ExtraExcludeRegex: []string{`\\Games\\`},
		IncludeRegex:      []string{`\\Portable\\`, `\\Tools\\`},
	}
	s := EverythingSearch(ev)
	for _, want := range []string{
		"file: ext:exe;ahk ",
		`!path:regex:"` + config.DefaultExcludeRegex + `"`,
		`!path:regex:"\\Games\\"`,
		`!path:regex:"` + config.InstalledRegex + `"`,
		`path:regex:"(\\Portable\\|\\Tools\\)"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("search missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(EverythingSearch(config.Everything{Extensions: []string{"exe"}}), " path:regex") {
		t.Error("empty include must not add a positive path filter")
	}
}

func TestFileSearch(t *testing.T) {
	ev := config.Everything{ExcludeRegex: `\\Temp\\`, ExtraExcludeRegex: []string{`\\cache\\`}}
	s := FileSearch(ev, `project"notes  list.cfg`)
	if !strings.HasPrefix(s, `file: "projectnotes" "list.cfg" `) {
		t.Errorf("words not quoted: %s", s)
	}
	if !strings.Contains(s, `!path:regex:"\\Temp\\"`) || !strings.Contains(s, `!path:regex:"\\cache\\"`) {
		t.Errorf("excludes missing: %s", s)
	}
	if strings.Contains(s, "ext:") || strings.Contains(s, "Program Files") {
		t.Errorf("file search must not restrict types or Program Files: %s", s)
	}
	open := OpenSearch(ev, `project"notes  list.cfg`)
	if strings.HasPrefix(open, "file:") || !strings.HasPrefix(open, `"projectnotes" "list.cfg" `) {
		t.Errorf("open search must include folders: %s", open)
	}
}

func TestQuery2AndList2(t *testing.T) {
	q := buildQuery2(0x1234, ipcReplyTest, ipcMatchPath, 20000, "file: ext:exe")
	le := binary.LittleEndian
	if le.Uint32(q[0:]) != 0x1234 || le.Uint32(q[8:]) != ipcMatchPath || le.Uint32(q[16:]) != 20000 || le.Uint32(q[20:]) != ipcRequestFullPathAndName {
		t.Errorf("query2 header wrong: %x", q[:28])
	}
	if len(q) != 28+2*(len("file: ext:exe")+1) || le.Uint16(q[len(q)-2:]) != 0 {
		t.Errorf("query2 search not null terminated")
	}

	reply := fakeList2(50, []string{`E:\Apps\a.exe`, `E:\Tools\ü.ahk`}, []uint32{0, 0})
	paths, total, err := parseList2(reply, false)
	if err != nil || total != 50 || len(paths) != 2 || paths[1] != (Hit{Path: `E:\Tools\ü.ahk`}) {
		t.Errorf("parse: %v %d %v", paths, total, err)
	}
	folder := fakeList2(1, []string{`E:\dir`}, []uint32{ipcItemFolder})
	if paths, _, _ := parseList2(folder, false); len(paths) != 0 {
		t.Errorf("folders must be skipped: %v", paths)
	}
	if paths, _, err := parseList2(folder, true); err != nil || len(paths) != 1 || paths[0] != (Hit{Path: `E:\dir`, Dir: true}) {
		t.Errorf("open search must keep folders, marked: %v %v", paths, err)
	}
	if _, _, err := parseList2(reply[:len(reply)-3], false); err == nil {
		t.Error("truncated reply must fail")
	}
}

const ipcReplyTest = 0x0A7A0001

func fakeList2(total int, paths []string, flags []uint32) []byte {
	le := binary.LittleEndian
	head := make([]byte, list2HeaderSize+item2Size*len(paths))
	le.PutUint32(head[0:], uint32(total))
	le.PutUint32(head[4:], uint32(len(paths)))
	le.PutUint32(head[12:], ipcRequestFullPathAndName)
	var data []byte
	for i, p := range paths {
		off := len(head) + len(data)
		le.PutUint32(head[list2HeaderSize+i*item2Size:], flags[i])
		le.PutUint32(head[list2HeaderSize+i*item2Size+4:], uint32(off))
		u := utf16.Encode([]rune(p))
		chunk := make([]byte, 4+2*len(u)+2)
		le.PutUint32(chunk, uint32(len(u)))
		for j, c := range u {
			le.PutUint16(chunk[4+2*j:], c)
		}
		data = append(data, chunk...)
	}
	return append(head, data...)
}
