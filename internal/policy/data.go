package policy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/open-policy-agent/opa/v1/util"
	"go.yaml.in/yaml/v3"

	"github.com/tarrball/specval/internal/yamlpath"
)

// Architecture is the baseline layer data, data.dpl.
type Architecture struct {
	// LayerOrder lists the layers from the top (client) to the bottom. The
	// utility layer is last.
	LayerOrder []string `json:"layer_order"`
	// Layers maps each layer to the layers that it can call.
	Layers map[string][]string `json:"layers"`
	// Suffix maps a layer to the end of the name of its services. A layer
	// without a suffix is not in the map.
	Suffix map[string]string `json:"suffix"`
}

// Service is one entry in the service registry, data.project.services.
type Service struct {
	Layer string `json:"layer"`
	// MayCall limits the services that this service can call. CalledBy
	// limits the services that can call this service. Each value is a layer
	// or a service. A nil list is no limit. An empty list that is not nil
	// permits no service.
	MayCall, CalledBy []string
	// Package tells where the code of the service is. It is "" if the entry
	// has no package, or a package that is not a string (SR009).
	Package string
}

// Architecture returns the baseline layer data.
func (e *Engine) Architecture() (Architecture, error) {
	var a Architecture
	err := roundTrip(e.data["dpl"], &a)
	return a, err
}

// Services returns the service registry, by service name. It returns an
// error only if the registry is not a mapping. An entry that is not a mapping,
// or that has no layer that is a string, has an empty Layer. The registry
// rules SR001 and SR004 report such an entry.
func (e *Engine) Services() (map[string]Service, error) {
	project, _ := e.data["project"].(map[string]any)
	raw, ok := project["services"]
	if !ok {
		return nil, nil
	}
	entries, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("data.project.services must be a mapping of service names to layers")
	}
	services := make(map[string]Service, len(entries))
	for name, v := range entries {
		var s Service
		if entry, ok := v.(map[string]any); ok {
			s.Layer, _ = entry["layer"].(string) // A layer that is not a string stays "".
			s.MayCall = limit(entry["may_call"])
			s.CalledBy = limit(entry["called_by"])
			s.Package, _ = entry["package"].(string) // A package that is not a string stays "".
		}
		services[name] = s
	}
	return services, nil
}

// limit returns the strings in a may_call or called_by value. It returns nil
// if the value is not a list. SR005 reports such a value, and a value in
// the list that is not a string.
func limit(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// DataFile returns the project data file that defines the top-level key.
// It returns "" if no data file defines the key.
func (e *Engine) DataFile(key string) string {
	return e.dataFile[key]
}

func (e *Engine) loadDataFile(path string, b []byte, project map[string]any) error {
	var node yaml.Node
	second, err := parseOne(b, &node)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if second > 0 {
		return fmt.Errorf("%s: line %d: a data file must have one YAML document", path, second)
	}
	var doc map[string]any
	if err := node.Decode(&doc); err != nil {
		// A key that is two times in a mapping also stops Decode. Then the
		// file is a mapping, and the YAML error tells which key it is.
		if root := &node; root.Kind == yaml.DocumentNode && len(root.Content) == 1 && root.Content[0].Kind == yaml.MappingNode {
			return fmt.Errorf("%s: %w", path, err)
		}
		return fmt.Errorf("%s: a data file must be a mapping: %w", path, err)
	}
	for k, v := range doc {
		if prev, ok := e.dataFile[k]; ok {
			return fmt.Errorf("two data files define the key %q: %s and %s", k, prev, path)
		}
		v, err := stringKeys(v, k)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		project[k] = v
		e.dataFile[k] = path
	}
	e.nodes[path] = &node
	return nil
}

// keyError reports two keys in a mapping that are the same as text, for
// example 0x1 and "1".
type keyError struct {
	key  string
	path string // the path of the key, for example "calls[0].1"
}

func (e keyError) Error() string {
	return fmt.Sprintf("two keys are %q as text (%s)", e.key, e.path)
}

// stringKeys returns v with each mapping key as a string. YAML permits a key
// such as 1, but JSON, and thus OPA, needs string keys. path is the path of v,
// for error messages. If two keys of a mapping are the same as text, so that
// one value would be lost, stringKeys returns a keyError.
func stringKeys(v any, path string) (any, error) {
	join := func(key string) string { return yamlpath.Key(path, key) }
	switch v := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			key := fmt.Sprint(k)
			if _, dup := out[key]; dup {
				return nil, keyError{key: key, path: join(key)}
			}
			conv, err := stringKeys(e, join(key))
			if err != nil {
				return nil, err
			}
			out[key] = conv
		}
		return out, nil
	case map[string]any:
		for k, e := range v {
			conv, err := stringKeys(e, join(k))
			if err != nil {
				return nil, err
			}
			v[k] = conv
		}
		return v, nil
	case []any:
		for i, e := range v {
			conv, err := stringKeys(e, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			v[i] = conv
		}
		return v, nil
	default:
		return v, nil
	}
}

// roundTrip converts values from the YAML decoder to the JSON types that OPA uses.
func roundTrip(in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return util.UnmarshalJSON(b, out)
}

// parseOne parses the first YAML document of b into node. If b has a second
// document, parseOne returns the line where it starts. Otherwise it returns
// 0. An empty b gives an empty node.
func parseOne(b []byte, node *yaml.Node) (int, error) {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(node); errors.Is(err, io.EOF) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	var next yaml.Node
	if err := dec.Decode(&next); errors.Is(err, io.EOF) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	return next.Line, nil
}
