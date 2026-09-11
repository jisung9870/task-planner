package domain

// Project is metadata for a body of work. Tasks reference it by slug rather
// than living under its directory: a field reference survives reorganisation
// and allows a task to be re-homed without moving files.
type Project struct {
	Slug   string   `yaml:"slug"`
	Name   string   `yaml:"name"`
	Status string   `yaml:"status,omitempty"` // active | paused | done
	Owner  string   `yaml:"owner,omitempty"`
	Due    Date     `yaml:"due,omitempty"`
	Tags   []string `yaml:"tags,omitempty"`
	Links  []string `yaml:"links,omitempty"`

	Extra map[string]any `yaml:",inline"`

	Body string `yaml:"-"`
	Path string `yaml:"-"`
}

func (p *Project) Display() string {
	if p.Name != "" {
		return p.Name
	}
	return p.Slug
}

func (p *Project) Active() bool { return p.Status == "" || p.Status == "active" }
