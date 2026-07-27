package job

// Path identifies how a job should be executed.
type Path int

const (
	// PathSimple uses a plain HTTP fetch with no browser rendering.
	PathSimple Path = iota
	// PathBrowser renders the page in headless Chrome.
	PathBrowser
)

func (p Path) String() string {
	if p == PathSimple {
		return "simple"
	}
	return "browser"
}

// Decide chooses an execution path for a job. A job takes the simple HTTP path
// when it explicitly opts out of rendering (fetchInstead) or targets a custom
// method endpoint; otherwise it is rendered in a browser so client-side JS runs.
func Decide(r *Request) Path {
	if r.FetchInstead {
		return PathSimple
	}
	if r.MethodEndpoint != "" {
		return PathSimple
	}
	return PathBrowser
}
