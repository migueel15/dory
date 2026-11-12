package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/migueel15/dory/node/localstore"
	"github.com/migueel15/dory/node/syncx"
)

// elem is one array element and the add tags that make it live.
type elem struct {
	value      any
	adds, rems map[string]bool
}

func (e *elem) live() []string {
	var out []string
	for t := range e.adds {
		if !e.rems[t] {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// state is what the importer needs to know about the operation log: the clock
// of every field (to give Base to a change), the live add tags of every array
// element (to remove them), and where the import device left off.
type state struct {
	device   string
	clocks   map[[3]string]string // kind, id, field
	elems    map[[3]string]map[string]*elem
	lastSeq  int64
	lastHLC  syncx.HLC
	maxEvent int64 // highest source event id already imported
	events   int   // imported event ops
}

func loadState(ctx context.Context, store *localstore.Store, device string) (*state, error) {
	_, ops, err := store.Log().Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	st := &state{device: device, clocks: map[[3]string]string{}, elems: map[[3]string]map[string]*elem{}}
	for _, o := range ops { // ordered by stamp
		f, err := o.Decode()
		if err != nil {
			return nil, err
		}
		if o.DeviceID == device {
			if o.Seq > st.lastSeq {
				st.lastSeq = o.Seq
			}
			if h, err := syncx.ParseHLC(o.HLC); err == nil && h.Compare(st.lastHLC) > 0 {
				st.lastHLC = h
			}
		}
		if o.Kind == syncx.KindEvent {
			if f.Event["device"] == device {
				st.events++
				if seq, ok := f.Event["seq"].(float64); ok && int64(seq) > st.maxEvent {
					st.maxEvent = int64(seq)
				}
			}
			continue
		}
		for k := range f.Set {
			st.clocks[[3]string{o.Entity, o.EntityID, k}] = o.HLC
		}
		for k, vs := range f.Adds {
			set := st.set(o.Entity, o.EntityID, k)
			for i, v := range vs {
				e := set.get(v)
				e.adds[fmt.Sprintf("%s#%04d", o.HLC, i)] = true
			}
		}
		for k, rs := range f.Rems {
			set := st.set(o.Entity, o.EntityID, k)
			for _, r := range rs {
				e := set.get(r.Elem)
				for _, t := range r.Tags {
					e.rems[t] = true
				}
			}
		}
	}
	return st, nil
}

type elemSet map[string]*elem

func (s elemSet) get(v any) *elem {
	c := canon(v)
	if s[c] == nil {
		s[c] = &elem{value: v, adds: map[string]bool{}, rems: map[string]bool{}}
	}
	return s[c]
}

func (st *state) set(kind, id, field string) elemSet {
	k := [3]string{kind, id, field}
	if st.elems[k] == nil {
		st.elems[k] = map[string]*elem{}
	}
	return st.elems[k]
}

// base records, in f, the clock of the value that a change to field replaces.
func (st *state) base(f *syncx.Fields, kind, id, field string) {
	if c, ok := st.clocks[[3]string{kind, id, field}]; ok {
		if f.Base == nil {
			f.Base = map[string]string{}
		}
		f.Base[field] = c
	}
}

// diffArray turns the difference between the live elements of an array field
// and want into element additions and removals.
func (st *state) diffArray(f *syncx.Fields, kind, id, field string, want any) {
	have := st.elems[[3]string{kind, id, field}]
	wanted := map[string]any{}
	var order []string
	if arr, ok := want.([]any); ok {
		for _, v := range arr {
			c := canon(v)
			if _, dup := wanted[c]; !dup {
				wanted[c] = v
				order = append(order, c)
			}
		}
	}
	for _, c := range order {
		if e := have[c]; e == nil || len(e.live()) == 0 {
			if f.Adds == nil {
				f.Adds = map[string][]any{}
			}
			f.Adds[field] = append(f.Adds[field], wanted[c])
		}
	}
	var gone []string
	for c, e := range have {
		if _, keep := wanted[c]; !keep && len(e.live()) > 0 {
			gone = append(gone, c)
		}
	}
	sort.Strings(gone)
	for _, c := range gone {
		if f.Rems == nil {
			f.Rems = map[string][]syncx.Removal{}
		}
		f.Rems[field] = append(f.Rems[field], syncx.Removal{Elem: have[c].value, Tags: have[c].live()})
	}
}

// stamp orders the pending operations by time, gives each a strictly greater
// stamp than the previous operation of the import device (so seq and stamp grow
// together) and numbers them.
func (st *state) stamp(plan []pending) []syncx.Op {
	sort.SliceStable(plan, func(i, j int) bool { return plan[i].wall < plan[j].wall })
	ops := make([]syncx.Op, 0, len(plan))
	last := st.lastHLC
	seq := st.lastSeq
	for _, p := range plan {
		h := syncx.HLC{Wall: p.wall, Device: st.device}
		if h.Wall < last.Wall || (h.Wall == last.Wall && h.Counter <= last.Counter) {
			h.Wall, h.Counter = last.Wall, last.Counter+1
			if h.Counter > 0xffff {
				h.Wall, h.Counter = h.Wall+1, 0
			}
		}
		last = syncx.HLC{Wall: h.Wall, Counter: h.Counter}
		seq++
		raw, _ := json.Marshal(p.fields)
		ops = append(ops, syncx.Op{DeviceID: st.device, Seq: seq, HLC: h.String(), Entity: p.entity, EntityID: p.id, Kind: p.kind, Fields: raw})
	}
	return ops
}
