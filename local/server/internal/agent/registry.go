package agent

import "time"

// Registry manages registered coding agents.
type Registry struct {
	agents []Agent
}

// NewRegistry creates an empty agent registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds an agent to the registry.
func (r *Registry) Register(a Agent) {
	r.agents = append(r.agents, a)
}

// Get returns the first agent with the given name, or nil.
func (r *Registry) Get(name string) Agent {
	for _, a := range r.agents {
		if a.Name() == name {
			return a
		}
	}
	return nil
}

// Watchers returns all registered agents that implement Watcher.
func (r *Registry) Watchers() []Watcher {
	var ws []Watcher
	for _, a := range r.agents {
		if w, ok := a.(Watcher); ok {
			ws = append(ws, w)
		}
	}
	return ws
}

// ProjectPathDiscoverers returns all registered agents that can discover
// candidate local project paths.
func (r *Registry) ProjectPathDiscoverers() []ProjectPathDiscoverer {
	var ds []ProjectPathDiscoverer
	for _, a := range r.agents {
		if d, ok := a.(ProjectPathDiscoverer); ok {
			ds = append(ds, d)
		}
	}
	return ds
}

// Resolver returns the SessionResolver for the first agent matching the given
// name, or nil.
func (r *Registry) Resolver(name string) SessionResolver {
	for _, a := range r.agents {
		if a.Name() == name {
			if sr, ok := a.(SessionResolver); ok {
				return sr
			}
		}
	}
	return nil
}

// LatestPollTime returns the most recent LastPollTime across all watchers.
// Returns the zero time if no watcher has polled yet.
func (r *Registry) LatestPollTime() time.Time {
	var latest time.Time
	for _, w := range r.Watchers() {
		if t := w.LastPollTime(); t.After(latest) {
			latest = t
		}
	}
	return latest
}

// Names returns the deduplicated names of all registered agents in registration order.
func (r *Registry) Names() []string {
	seen := make(map[string]struct{})
	var out []string
	for _, a := range r.agents {
		name := a.Name()
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	return out
}
