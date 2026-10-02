package adminhttp

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hollis-labs/go-envelopes/admin"
)

// object rejects duplicate members rather than accepting last-key-wins JSON.
func object(b []byte) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, failure(admin.MalformedInput)
	}
	members := map[string]json.RawMessage{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return nil, failure(admin.MalformedInput)
		}
		key, ok := token.(string)
		if !ok {
			return nil, failure(admin.MalformedInput)
		}
		if _, exists := members[key]; exists {
			return nil, failure(admin.MalformedInput)
		}
		var raw json.RawMessage
		if err = d.Decode(&raw); err != nil {
			return nil, failure(admin.MalformedInput)
		}
		members[key] = raw
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') {
		return nil, failure(admin.MalformedInput)
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, failure(admin.MalformedInput)
	}
	return members, nil
}
func decodeCommand(r *http.Request, operation string, maxBytes int64) (admin.Command, error) {
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
		return admin.Command{}, failure(admin.MalformedInput)
	}
	if r.Body == nil {
		return admin.Command{}, failure(admin.MalformedInput)
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil || int64(len(b)) > maxBytes || !utf8.Valid(b) || !pairedSurrogates(b) {
		return admin.Command{}, failure(admin.MalformedInput)
	}
	members, err := object(b)
	if err != nil {
		return admin.Command{}, err
	}
	var command admin.Command
	if err = json.Unmarshal(members["revision"], &command.Revision); err != nil || strings.TrimSpace(command.Revision) == "" {
		return admin.Command{}, failure(admin.MalformedInput)
	}
	command.Set = map[string]admin.Scalar{}
	if operation == "reset" {
		if len(members) != 2 || members["keys"] == nil {
			return admin.Command{}, failure(admin.MalformedInput)
		}
		if err = stringArray(members["keys"], &command.Unset); err != nil || command.Unset == nil {
			return admin.Command{}, failure(admin.MalformedInput)
		}
	} else {
		if len(members) != 3 || members["set"] == nil || members["unset"] == nil {
			return admin.Command{}, failure(admin.MalformedInput)
		}
		set, err := object(members["set"])
		if err != nil {
			return admin.Command{}, err
		}
		for key, raw := range set {
			var v admin.Scalar
			if err = json.Unmarshal(raw, &v); err != nil {
				return admin.Command{}, failure(admin.MalformedInput)
			}
			command.Set[key] = v
		}
		if err = stringArray(members["unset"], &command.Unset); err != nil || command.Unset == nil {
			return admin.Command{}, failure(admin.MalformedInput)
		}
	}
	seen := map[string]bool{}
	for key := range command.Set {
		seen[key] = true
	}
	for _, key := range command.Unset {
		if seen[key] {
			return admin.Command{}, failure(admin.MalformedInput)
		}
		seen[key] = true
	}
	return command, nil
}
func precondition(r *http.Request, current string) *admin.Failure {
	values := r.Header.Values("If-Match")
	if len(values) == 0 {
		return failure(admin.PreconditionRequired)
	}
	if len(values) != 1 {
		return failure(admin.MalformedInput)
	}
	tag := strings.TrimSpace(values[0])
	if tag == "" {
		return failure(admin.PreconditionRequired)
	}
	weak := strings.HasPrefix(tag, "W/")
	if weak {
		tag = strings.TrimPrefix(tag, "W/")
	}
	if len(tag) < 2 || tag[0] != '"' || tag[len(tag)-1] != '"' {
		return failure(admin.MalformedInput)
	}
	for _, c := range tag[1 : len(tag)-1] {
		if c < 33 || c > 126 || c == '"' {
			return failure(admin.MalformedInput)
		}
	}
	if weak || tag != current {
		return failure(admin.ValueConflict)
	}
	return nil
}

func stringArray(b []byte, result *[]string) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil || raw == nil {
		return failure(admin.MalformedInput)
	}
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		if len(item) == 0 || item[0] != '"' {
			return failure(admin.MalformedInput)
		}
		var value string
		if err := json.Unmarshal(item, &value); err != nil {
			return failure(admin.MalformedInput)
		}
		values = append(values, value)
	}
	*result = values
	return nil
}

// encoding/json replaces isolated surrogate escapes with U+FFFD. Reject them
// rather than silently changing the caller's string or secret replacement.
func pairedSurrogates(b []byte) bool {
	inString := false
	for i := 0; i < len(b); i++ {
		if b[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || b[i] != '\\' {
			continue
		}
		i++
		if i >= len(b) {
			return false
		}
		if b[i] != 'u' {
			continue
		}
		if i+4 >= len(b) {
			return false
		}
		code, err := strconv.ParseUint(string(b[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if code >= 0xdc00 && code <= 0xdfff {
			return false
		}
		if code >= 0xd800 && code <= 0xdbff {
			if i+6 >= len(b) || b[i+1] != '\\' || b[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(b[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
