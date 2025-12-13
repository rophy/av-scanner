package drivers

import (
	"testing"
)

func TestTmVirusFoundRegex(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantPath string
		wantMatch bool
	}{
		{
			name:      "virus found log line",
			line:      `2025-11-21 13:53:06.726130: [ds_am/4] | [SCTRL] (0000-0000-0000, /home/ubuntu/xxxx.file) virus found: 2, act_1st=2, act_2nd=255, act_1st_error_code=0 | scanctrl_vmpd_module.cpp:1538:scanctrl_determine_send_dispatch_result | F7E01:1784DB:4451::`,
			wantPath:  "/home/ubuntu/xxxx.file",
			wantMatch: true,
		},
		{
			name:      "path with spaces",
			line:      `2025-11-21 13:53:06.726130: [ds_am/4] | [SCTRL] (0000-0000-0000, /tmp/av-scanner/test file.txt) virus found: 1`,
			wantPath:  "/tmp/av-scanner/test file.txt",
			wantMatch: true,
		},
		{
			name:      "no virus",
			line:      `2025-11-21 13:53:06.726130: [ds_am/4] | [SCTRL] scan completed successfully`,
			wantPath:  "",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := tmVirusFoundRegex.FindStringSubmatch(tt.line)
			if tt.wantMatch {
				if matches == nil {
					t.Errorf("expected match but got none")
					return
				}
				if len(matches) < 2 {
					t.Errorf("expected capture group, got %v", matches)
					return
				}
				if matches[1] != tt.wantPath {
					t.Errorf("got path %q, want %q", matches[1], tt.wantPath)
				}
			} else {
				if matches != nil {
					t.Errorf("expected no match but got %v", matches)
				}
			}
		})
	}
}
