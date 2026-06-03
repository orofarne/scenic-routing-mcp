-- Defines which OSM tags to import into the feature table.
-- { key = "leisure" }                     -- import all objects where this key exists
-- { key = "waterway", values = {"river"}} -- import only listed values
--
-- See https://wiki.openstreetmap.org/wiki/Map_features for tag reference.
-- After changing this file, run a full re-import (import.sh).

return {
    -- Green space and recreation
    { key = "leisure" },
    { key = "natural" },
    { key = "landuse", values = {
        "forest", "wood", "grass", "meadow", "greenfield",
        "recreation_ground", "village_green", "allotments",
    }},

    -- Waterways: linear features and water infrastructure
    { key = "waterway", values = {
        "river", "canal", "stream", "drain", "ditch",
        "dock", "weir", "dam", "waterfall", "lock_gate",
        "boatyard", "fuel",
    }},

    -- Mooring permission/type on waterway features
    { key = "mooring" },

    -- Man-made water and harbour infrastructure
    { key = "man_made", values = {"pier", "breakwater", "lock_gate", "dam", "weir", "bridge"} },

    -- Points of interest
    { key = "amenity" },
    { key = "tourism" },
    { key = "historic" },

    -- Tunnels (pedestrian underpasses, road/rail tunnels)
    { key = "tunnel" },

    -- Named walking/cycling routes (Thames Path, Capital Ring, etc.)
    { key = "route", values = {"foot", "hiking", "walking", "bicycle"} },

    -- Transit stops (point objects on the transport network, not road segments)
    { key = "highway",          values = {"bus_stop"} },
    { key = "railway",          values = {"tram_stop", "station", "halt"} },
    { key = "public_transport", values = {"stop_position", "platform"} },
}
