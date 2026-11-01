// Copyright 2026 OpenCloud GmbH <mail@opencloud.eu>
// SPDX-License-Identifier: Apache-2.0

// Package preconditions implements entity-tag conditions for storage mutations.
package preconditions

import (
	"net/http"
	"strings"

	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/errtypes"
)

const (
	IfMatch     = "if-match"
	IfNoneMatch = "if-none-match"
)

// Conditions carries both conditions independently of the CS3 upload options oneof.
// The opaque keys use plain-encoded HTTP entity-tag field values.
type Conditions struct {
	IfMatch     string
	IfNoneMatch string
}

func (c Conditions) Empty() bool { return c.IfMatch == "" && c.IfNoneMatch == "" }

func FromHTTP(h http.Header) (Conditions, error) {
	c := Conditions{IfMatch: strings.Join(h.Values("If-Match"), ","), IfNoneMatch: strings.Join(h.Values("If-None-Match"), ",")}
	for _, key := range []string{"If-Match", "If-None-Match"} {
		if values, ok := h[http.CanonicalHeaderKey(key)]; ok && strings.TrimSpace(strings.Join(values, ",")) == "" {
			return c, errtypes.BadRequest("empty " + key)
		}
	}
	return c, c.Validate()
}

func FromOpaque(o *types.Opaque) Conditions {
	read := func(key string) string {
		if e := o.GetMap()[key]; e != nil && e.Decoder == "plain" {
			return string(e.Value)
		}
		return ""
	}
	return Conditions{IfMatch: read(IfMatch), IfNoneMatch: read(IfNoneMatch)}
}

func (c Conditions) Opaque(o *types.Opaque) *types.Opaque {
	if o == nil {
		o = &types.Opaque{}
	}
	if o.Map == nil {
		o.Map = map[string]*types.OpaqueEntry{}
	}
	for key, value := range c.Metadata() {
		if value != "" {
			o.Map[key] = &types.OpaqueEntry{Decoder: "plain", Value: []byte(value)}
		}
	}
	return o
}

func FromMetadata(m map[string]string) Conditions {
	return Conditions{IfMatch: m[IfMatch], IfNoneMatch: m[IfNoneMatch]}
}

func (c Conditions) Metadata() map[string]string {
	return map[string]string{IfMatch: c.IfMatch, IfNoneMatch: c.IfNoneMatch}
}

func (c Conditions) Validate() error {
	for _, field := range []string{c.IfMatch, c.IfNoneMatch} {
		if field != "" {
			if _, _, err := parse(field); err != nil {
				return err
			}
		}
	}
	return nil
}

// Evaluate uses strong comparison for If-Match and weak comparison for If-None-Match.
func (c Conditions) Evaluate(exists bool, etag string) error {
	for _, condition := range []struct {
		value string
		match bool
	}{{c.IfMatch, true}, {c.IfNoneMatch, false}} {
		if condition.value == "" {
			continue
		}
		wildcard, tags, err := parse(condition.value)
		if err != nil {
			return err
		}
		matched := exists && wildcard
		for _, tag := range tags {
			if exists && ((!condition.match && weak(tag) == weak(etag)) || (condition.match && !strings.HasPrefix(tag, "W/") && !strings.HasPrefix(etag, "W/") && tag == etag)) {
				matched = true
			}
		}
		if matched != condition.match {
			return errtypes.Aborted("entity-tag precondition failed")
		}
	}
	return nil
}

func weak(tag string) string { return strings.TrimPrefix(tag, "W/") }

// Commas inside quoted opaque tags are data, not list separators (RFC 9110 §8.8.3).
func parse(value string) (bool, []string, error) {
	value = strings.TrimSpace(value)
	if value == "*" {
		return true, nil, nil
	}
	var tags []string
	for value != "" {
		value = strings.TrimLeft(value, " \t,")
		if value == "" {
			break
		}
		start := value
		if strings.HasPrefix(value, "W/") {
			value = value[2:]
		}
		if len(value) == 0 || value[0] != '"' {
			return false, nil, errtypes.BadRequest("invalid entity-tag condition")
		}
		value = value[1:]
		i := 0
		for i < len(value) && value[i] != '"' {
			if value[i] < 0x21 || value[i] == 0x7f {
				return false, nil, errtypes.BadRequest("invalid entity tag")
			}
			i++
		}
		if i == len(value) {
			return false, nil, errtypes.BadRequest("unterminated entity tag")
		}
		value = value[i+1:]
		tags = append(tags, start[:len(start)-len(value)])
		value = strings.TrimLeft(value, " \t")
		if value != "" && value[0] != ',' {
			return false, nil, errtypes.BadRequest("invalid entity-tag list")
		}
	}
	if len(tags) == 0 {
		return false, nil, errtypes.BadRequest("empty entity-tag list")
	}
	return false, tags, nil
}
