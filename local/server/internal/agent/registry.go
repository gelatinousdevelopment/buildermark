package agent

// Registry manages registered coding agents.
type Registry struct {
	agents map[string]Agent
	order  []string // preserves registration order
}

// NewRegistry creates an empty agent registry.
func NewRegistry() *Registry {
	return &Registry{agents: make(map[string]Agent)}
}

// Register adds an agent to the registry.
func (r *Registry) Register(a Agent) {
	name := a.Name()
	if _, exists := r.agents[name]; !exists {
		r.order = append(r.order, name)
	}
	r.agents[name] = a
}

// Get returns the agent with the given name, or nil.
func (r *Registry) Get(name string) Agent {
	return r.agents[name]
}

// Watchers returns all registered agents that implement Watcher.
func (r *Registry) Watchers() []Watcher {
	var ws []Watcher
	for _, name := range r.order {
		if w, ok := r.agents[name].(Watcher); ok {
			ws = append(ws, w)
		}
	}
	return ws
}

// ProjectPathDiscoverers returns all registered agents that can discover
// candidate local project paths.
func (r *Registry) ProjectPathDiscoverers() []ProjectPathDiscoverer {
	var ds []ProjectPathDiscoverer
	for _, name := range r.order {
		if d, ok := r.agents[name].(ProjectPathDiscoverer); ok {
			ds = append(ds, d)
		}
	}
	return ds
}

// Resolver returns the SessionResolver for the given agent name, or nil.
func (r *Registry) Resolver(name string) SessionResolver {
	a, ok := r.agents[name]
	if !ok {
		return nil
	}
	sr, ok := a.(SessionResolver)
	if !ok {
		return nil
	}
	return sr
}

// Names returns the names of all registered agents in registration order.
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}
