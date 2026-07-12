// Package clustering is a require()-able host module exposing unsupervised
// clustering algorithms (k-means, DBSCAN, agglomerative/hierarchical, and
// mean shift) to .lsc code. Data is passed as an array of points, where a
// point is itself an array of numbers; results report 1-based cluster ids
// so they index naturally from Lua.
package clustering

import (
	"math/rand"

	"github.com/hilthontt/luascript/internal/vm"
)

// RegisterClusteringPreload installs the loader under package.preload so
// the module table is built lazily on the first require("clustering").
func RegisterClusteringPreload(v *vm.VM) {
	vm.RegisterPreload(v, "clustering", clusteringLoader)
}

func clusteringLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := vm.NewTable(0, 6)
	mod.Set("VERSION", "0.1.0")
	mod.Set("kmeans", &vm.GoFunc{Name: "clustering.kmeans", Fn: clKmeans})
	mod.Set("dbscan", &vm.GoFunc{Name: "clustering.dbscan", Fn: clDBSCAN})
	mod.Set("hierarchical", &vm.GoFunc{Name: "clustering.hierarchical", Fn: clHierarchical})
	mod.Set("meanshift", &vm.GoFunc{Name: "clustering.meanshift", Fn: clMeanShift})
	return []vm.Value{mod}
}

// clKmeans: clustering.kmeans(data, k [, opts]) -> { centroids, assignments, iterations }
// opts: { delta = 0.001, maxIter = 100, seed = 1 }
func clKmeans(_ *vm.VM, args []vm.Value) []vm.Value {
	data := tableToPoints("clustering.kmeans", vm.TableArg("clustering.kmeans", 1, args))
	k := int(vm.IntArg("clustering.kmeans", 2, args))
	opts := optTable(args, 3)
	if k < 1 || k > len(data) {
		panic(vm.Errorf("clustering.kmeans: k must be between 1 and #data (%d), got %d", len(data), k))
	}
	requireUniformDims("clustering.kmeans", data)

	delta := optFloat(opts, "delta", 0.001)
	maxIter := int(optInt(opts, "maxIter", 100))
	seed := optInt(opts, "seed", 1)
	rng := rand.New(rand.NewSource(seed))

	res := KMeans(data, k, maxIter, delta, rng)

	out := vm.NewTable(0, 3)
	out.Set("centroids", pointsToTable(res.Centers))
	out.Set("assignments", labelsToTable(res.Assignments, true)) // +1: 0-based -> 1-based
	out.Set("iterations", int64(res.Iterations))
	return []vm.Value{out}
}

// clDBSCAN: clustering.dbscan(data, eps, minPts) -> assignments
// Cluster ids are 1-based; 0 marks noise points.
func clDBSCAN(_ *vm.VM, args []vm.Value) []vm.Value {
	data := tableToPoints("clustering.dbscan", vm.TableArg("clustering.dbscan", 1, args))
	eps := vm.FloatArg("clustering.dbscan", 2, args)
	minPts := int(vm.IntArg("clustering.dbscan", 3, args))
	requireUniformDims("clustering.dbscan", data)
	if eps <= 0 {
		panic(vm.Errorf("clustering.dbscan: eps must be positive"))
	}

	labels := DBSCAN(data, eps, minPts)
	return []vm.Value{labelsToTable(labels, false)} // already 1-based / 0=noise
}

// clHierarchical: clustering.hierarchical(data, k [, opts]) -> assignments
// opts: { linkage = "average" | "single" | "complete" }
func clHierarchical(_ *vm.VM, args []vm.Value) []vm.Value {
	data := tableToPoints("clustering.hierarchical", vm.TableArg("clustering.hierarchical", 1, args))
	k := int(vm.IntArg("clustering.hierarchical", 2, args))
	opts := optTable(args, 3)
	if k < 1 || k > len(data) {
		panic(vm.Errorf("clustering.hierarchical: k must be between 1 and #data (%d), got %d", len(data), k))
	}
	requireUniformDims("clustering.hierarchical", data)

	linkage := ParseLinkage(optString(opts, "linkage", "average"))
	labels := Hierarchical(data, k, linkage)
	return []vm.Value{labelsToTable(labels, false)} // already 1-based
}

// clMeanShift: clustering.meanshift(data, bandwidth [, opts]) -> { centroids, assignments, iterations }
// opts: { maxIter = 100 }
func clMeanShift(_ *vm.VM, args []vm.Value) []vm.Value {
	data := tableToPoints("clustering.meanshift", vm.TableArg("clustering.meanshift", 1, args))
	bandwidth := vm.FloatArg("clustering.meanshift", 2, args)
	opts := optTable(args, 3)
	requireUniformDims("clustering.meanshift", data)
	if bandwidth <= 0 {
		panic(vm.Errorf("clustering.meanshift: bandwidth must be positive"))
	}

	maxIter := int(optInt(opts, "maxIter", 100))
	res := MeanShift(data, bandwidth, maxIter)

	out := vm.NewTable(0, 3)
	out.Set("centroids", pointsToTable(res.Centers))
	out.Set("assignments", labelsToTable(res.Assignments, false)) // already 1-based
	out.Set("iterations", int64(res.Iterations))
	return []vm.Value{out}
}

// Conversion helpers

// tableToPoints reads an array-of-arrays Lua table into []Point. Each inner
// table must hold only numbers.
func tableToPoints(name string, t *vm.Table) []Point {
	n := int(t.Len())
	if n == 0 {
		panic(vm.Errorf("%s: data must be a non-empty array of points", name))
	}
	pts := make([]Point, 0, n)
	for i := 1; i <= n; i++ {
		row, ok := t.Get(int64(i)).(*vm.Table)
		if !ok {
			panic(vm.Errorf("%s: data[%d] must be an array of numbers", name, i))
		}
		m := int(row.Len())
		if m == 0 {
			panic(vm.Errorf("%s: data[%d] must have at least one dimension", name, i))
		}
		entry := make([]float64, m)
		for j := 1; j <= m; j++ {
			f, ok := vm.ToFloat(row.Get(int64(j)))
			if !ok {
				panic(vm.Errorf("%s: data[%d][%d] must be a number", name, i, j))
			}
			entry[j-1] = f
		}
		pts = append(pts, Point{Entry: entry})
	}
	return pts
}

// requireUniformDims rejects ragged input — every algorithm here assumes a
// shared dimensionality, and distanceSqTo would otherwise read out of range.
func requireUniformDims(name string, pts []Point) {
	dims := len(pts[0].Entry)
	for i := range pts {
		if len(pts[i].Entry) != dims {
			panic(vm.Errorf("%s: all points must share the same dimensionality (data[1] has %d, data[%d] has %d)",
				name, dims, i+1, len(pts[i].Entry)))
		}
	}
}

func pointsToTable(pts []Point) *vm.Table {
	out := vm.NewTable(len(pts), 0)
	for i, p := range pts {
		row := vm.NewTable(len(p.Entry), 0)
		for j, v := range p.Entry {
			row.Set(int64(j+1), v)
		}
		out.Set(int64(i+1), row)
	}
	return out
}

// labelsToTable converts a label slice to a Lua array. When bump is true
// the labels are 0-based and are shifted to 1-based for Lua ergonomics.
func labelsToTable(labels []int, bump bool) *vm.Table {
	out := vm.NewTable(len(labels), 0)
	for i, l := range labels {
		if bump {
			l++
		}
		out.Set(int64(i+1), int64(l))
	}
	return out
}

// Option-table helpers

func optTable(args []vm.Value, n int) *vm.Table {
	if n < 1 || n > len(args) || args[n-1] == nil {
		return nil
	}
	if t, ok := args[n-1].(*vm.Table); ok {
		return t
	}
	return nil
}

func optFloat(opts *vm.Table, key string, dflt float64) float64 {
	if opts == nil {
		return dflt
	}
	if f, ok := vm.ToFloat(opts.Get(key)); ok {
		return f
	}
	return dflt
}

func optInt(opts *vm.Table, key string, dflt int64) int64 {
	if opts == nil {
		return dflt
	}
	if i, ok := vm.ToInteger(opts.Get(key)); ok {
		return i
	}
	return dflt
}

func optString(opts *vm.Table, key, dflt string) string {
	if opts == nil {
		return dflt
	}
	if s, ok := opts.Get(key).(string); ok {
		return s
	}
	return dflt
}
