package media

import (
	"context"
	"errors"
	"testing"
)

type fakeTool struct {
	probe Info
	err   error
	calls *int
}

func (f fakeTool) Probe(context.Context, string) (Info, error) {
	*f.calls++
	return f.probe, f.err
}

func (f fakeTool) Thumbnail(context.Context, string, string, int) error {
	*f.calls++
	return f.err
}

func TestChain(t *testing.T) {
	broken := errors.New("broken file")
	tests := []struct {
		name              string
		first, then       error
		noFirst, noThen   bool
		want              error
		firstN, thenN     int
		wantInfoFromFirst bool
	}{
		{name: "first answers", first: nil, then: nil, want: nil, firstN: 1, thenN: 0, wantInfoFromFirst: true},
		{name: "first cannot, then does", first: ErrUnavailable, then: nil, want: nil, firstN: 1, thenN: 1},
		{name: "both missing", first: ErrUnavailable, then: ErrUnavailable, want: ErrUnavailable, firstN: 1, thenN: 1},
		{name: "a real error beats a missing tool", first: broken, then: ErrUnavailable, want: broken, firstN: 1, thenN: 1},
		{name: "the fallback's error wins otherwise", first: ErrUnavailable, then: broken, want: broken, firstN: 1, thenN: 1},
		{name: "no first", noFirst: true, then: nil, want: nil, thenN: 1},
		{name: "no fallback", first: broken, noThen: true, want: broken, firstN: 1},
	}
	for _, tc := range tests {
		for _, op := range []string{"probe", "thumbnail"} {
			var fn, tn int
			c := Chain{}
			if !tc.noFirst {
				c.First = fakeTool{probe: Info{Width: 1}, err: tc.first, calls: &fn}
			}
			if !tc.noThen {
				c.Then = fakeTool{probe: Info{Width: 2}, err: tc.then, calls: &tn}
			}
			var err error
			var info Info
			if op == "probe" {
				info, err = c.Probe(context.Background(), "x.mp4")
			} else {
				err = c.Thumbnail(context.Background(), "x.mp4", "x.jpg", 64)
			}
			if !errors.Is(err, tc.want) || (tc.want == nil && err != nil) || fn != tc.firstN || tn != tc.thenN {
				t.Errorf("%s (%s): err %v, calls %d/%d; want %v, %d/%d", tc.name, op, err, fn, tn, tc.want, tc.firstN, tc.thenN)
			}
			if op == "probe" && err == nil && (info.Width == 1) != tc.wantInfoFromFirst {
				t.Errorf("%s: info %+v from the wrong tool", tc.name, info)
			}
		}
	}
}
