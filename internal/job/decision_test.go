package job

import "testing"

func TestDecide(t *testing.T) {
	tests := []struct {
		name string
		req  *Request
		want Path
	}{
		{"default renders in browser", &Request{URL: "https://a.com"}, PathBrowser},
		{"fetchInstead takes simple path", &Request{URL: "https://a.com", FetchInstead: true}, PathSimple},
		{"method endpoint takes simple path", &Request{URL: "https://a.com", MethodEndpoint: "https://api.x"}, PathSimple},
		{"GET without flags still renders", &Request{URL: "https://a.com", Method: "GET"}, PathBrowser},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decide(tt.req); got != tt.want {
				t.Errorf("Decide() = %v, want %v", got, tt.want)
			}
		})
	}
}
