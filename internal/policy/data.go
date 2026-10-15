package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/open-policy-agent/opa/v1/util"
	"go.yaml.in/yaml/v3"
)

// Architecture is the baseline layer data, data.dpl.
type Architecture struct {
	// LayerOrder lists the layers from the top (manager) to the bottom.
	LayerOrder []string `json:"layer_order"`
	// Layers maps each layer to the layers that it can call.
	Layers map[string][]string `json:"layers"`
	// Suffix maps each layer to the end of the name of its services.
	Suffix map[string]string `json:"suffix"`
}

// Service is one entry in the service registry, data.project.services.
type Service struct {
	Layer string `json:"layer"`
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
		}
		services[name] = s
	}
	return services, nil
}

// DataFile returns the project data file that defines the top-level key.
// It returns "" if no data file defines the key.
func (e *Engine) DataFile(key string) string {
	return e.dataFile[key]
}

func (e *Engine) loadDataFile(path string, project map[string]any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(b, &node); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	var doc map[string]any
	if err := node.Decode(&doc); err != nil {
		return fmt.Errorf("%s: a data file must be a mapping: %w", path, err)
	}
	for k, v := range doc {
		if prev, ok := e.dataFile[k]; ok {
			return fmt.Errorf("two data files define the key %q: %s and %s", k, prev, path)
		}
		project[k] = stringKeys(v)
		e.dataFile[k] = path
	}
	e.nodes[path] = &node
	return nil
}

// stringKeys returns v with each mapping key as a string. YAML permits a key
// such as 1, but JSON, and thus OPA, needs string keys.
func stringKeys(v any) any {
	switch v := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[fmt.Sprint(k)] = stringKeys(e)
		}
		return out
	case map[string]any:
		for k, e := range v {
			v[k] = stringKeys(e)
		}
		return v
	case []any:
		for i, e := range v {
			v[i] = stringKeys(e)
		}
		return v
	default:
		return v
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
