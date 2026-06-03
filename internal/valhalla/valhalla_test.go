package valhalla

import "testing"

func TestMakeLocations(t *testing.T) {
	t.Run("two waypoints: both are breaks", func(t *testing.T) {
		locs := makeLocations([][2]float64{{51.5, -0.1}, {51.6, 0.1}})
		if len(locs) != 2 {
			t.Fatalf("len = %d, want 2", len(locs))
		}
		if locs[0].Type != "break" {
			t.Errorf("locs[0].Type = %q, want break", locs[0].Type)
		}
		if locs[1].Type != "break" {
			t.Errorf("locs[1].Type = %q, want break", locs[1].Type)
		}
	})

	t.Run("three waypoints: intermediate is through", func(t *testing.T) {
		locs := makeLocations([][2]float64{{0, 0}, {1, 1}, {2, 2}})
		if locs[0].Type != "break" {
			t.Errorf("locs[0].Type = %q, want break", locs[0].Type)
		}
		if locs[1].Type != "through" {
			t.Errorf("locs[1].Type = %q, want through", locs[1].Type)
		}
		if locs[2].Type != "break" {
			t.Errorf("locs[2].Type = %q, want break", locs[2].Type)
		}
	})

	t.Run("four waypoints: both intermediates are through", func(t *testing.T) {
		locs := makeLocations([][2]float64{{0, 0}, {1, 0}, {2, 0}, {3, 0}})
		for _, i := range []int{1, 2} {
			if locs[i].Type != "through" {
				t.Errorf("locs[%d].Type = %q, want through", i, locs[i].Type)
			}
		}
	})

	t.Run("lat/lon are preserved", func(t *testing.T) {
		locs := makeLocations([][2]float64{{51.5, -0.116}})
		if locs[0].Lat != 51.5 || locs[0].Lon != -0.116 {
			t.Errorf("locs[0] = %+v, want {51.5 -0.116 break}", locs[0])
		}
	})
}
