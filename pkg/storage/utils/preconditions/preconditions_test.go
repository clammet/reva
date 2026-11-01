package preconditions

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEntityTagConditions(t *testing.T) {
	for _, tt := range []struct {
		name, match, none, etag string
		exists, pass            bool
	}{
		{"unconditional missing", "", "", "", false, true},
		{"match missing", `"a"`, "", "", false, false},
		{"wildcard missing", "*", "", "", false, false},
		{"wildcard exists", "*", "", `"a"`, true, true},
		{"matching", `"a"`, "", `"a"`, true, true},
		{"stale", `"b"`, "", `"a"`, true, false},
		{"list", `"b", "a"`, "", `"a"`, true, true},
		{"weak request", `W/"a"`, "", `"a"`, true, false},
		{"weak current", `"a"`, "", `W/"a"`, true, false},
		{"comma in tag", `"a,b", "c"`, "", `"a,b"`, true, true},
		{"exclusive missing", "", "*", "", false, true},
		{"exclusive exists", "", "*", `"a"`, true, false},
		{"none matching", "", `"a"`, `"a"`, true, false},
		{"none weak request", "", `W/"a"`, `"a"`, true, false},
		{"none weak current", "", `"a"`, `W/"a"`, true, false},
		{"none list", "", `"b", W/"a"`, `"a"`, true, false},
		{"none different", "", `"b"`, `"a"`, true, true},
		{"both pass", `"a"`, `"b"`, `"a"`, true, true},
		{"both fail", `"a"`, `"a"`, `"a"`, true, false},
		{"whitespace", " \t\"b\",\t\"a\" ", "", `"a"`, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := (Conditions{IfMatch: tt.match, IfNoneMatch: tt.none}).Evaluate(tt.exists, tt.etag)
			if tt.pass {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestInvalidConditions(t *testing.T) {
	for _, value := range []string{`abc`, `"abc`, `*, "a"`, `"a", *`, `W/a`, `"a" "b"`, "\"a\nb\"", " ", ",,,"} {
		t.Run(value, func(t *testing.T) { require.Error(t, (Conditions{IfMatch: value}).Validate()) })
	}
	_, err := FromHTTP(http.Header{"If-Match": {""}})
	require.Error(t, err)
}

func TestRepeatedHeadersAndOpaque(t *testing.T) {
	c, err := FromHTTP(http.Header{"If-Match": {`"a,b"`, `"c"`}, "If-None-Match": {`W/"d"`}})
	require.NoError(t, err)
	require.NoError(t, c.Evaluate(true, `"c"`))
	require.Equal(t, c, FromOpaque(c.Opaque(nil)))
	require.Equal(t, c, FromMetadata(c.Metadata()))
}
