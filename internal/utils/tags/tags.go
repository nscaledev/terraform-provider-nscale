/*
Copyright 2026 Nscale

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package tags holds the provider's own representation of a resource tag,
// together with conversions to and from the generated SDK types.
//
// The SDK publishes each service from a bundled spec with every $ref
// dereferenced, so there is no shared package and every service declares its
// own structurally identical Tag. Those types are distinct to the compiler and
// so are not interchangeable. Rather than fan that split out across every model
// and helper, the provider owns one Tag and converts at the SDK boundary.
package tags

// Tag is the provider's tag representation.
//
// Its layout — field names, field types and struct tags — must stay identical to
// the SDK's per-service Tag types, because SDKTag matches on exactly that shape
// and the conversions below rely on the two being convertible.
type Tag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// List mirrors the SDK's TagList. Declared as an alias rather than a defined
// type for the same reason the SDK does: it keeps *List and *[]Tag identical, so
// a metadata field typed *TagList can be passed to a *[]Tag parameter without a
// conversion at every call site.
type List = []Tag

// SDKTag is satisfied by any generated tag type whose underlying struct matches
// Tag.
//
// Constraining structurally rather than listing each service's Tag keeps this
// package free of SDK imports, and means adding a service needs no change here.
// The trade-off is deliberate: if a tag's shape changes upstream, every
// conversion below fails to compile rather than silently dropping a field.
type SDKTag interface {
	~struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
}

// FromAPI converts a service package's tags into the provider's own.
//
// Nil in, nil out: the SDK models tags as an optional pointer, and a resource
// with no tags must stay distinguishable from one with an empty set.
func FromAPI[T SDKTag](in *[]T) *List {
	if in == nil {
		return nil
	}

	out := make(List, len(*in))
	for i, tag := range *in {
		out[i] = Tag(tag)
	}

	return &out
}

// ToAPI is the inverse of FromAPI, for building request bodies. T cannot be
// inferred from the return value, so callers name the service's tag type
// explicitly — for example tags.ToAPI[regionapi.Tag](tagList).
func ToAPI[T SDKTag](in *List) *[]T {
	if in == nil {
		return nil
	}

	out := make([]T, len(*in))
	for i, tag := range *in {
		out[i] = T(tag)
	}

	return &out
}
