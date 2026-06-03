-- osm2pgsql flex output for scenic routing.
-- Tag filtering is driven entirely by osm_tags.lua — no hardcoded categories here.
--
-- Three raw tables, one per OSM primitive type:
--   raw_node  — nodes
--   raw_line  — open ways  (rivers, canals, ...)
--   raw_poly  — closed ways + relations (parks, forests, water bodies, ...)
--
-- post-import.sql merges them into the single `feature` table.

-- ── Load tag config ───────────────────────────────────────────────────────────

local config = dofile(os.getenv('OSM_TAGS_CONFIG') or '/scripts/osm_tags.lua')

-- Build tag_filter: key → ANY sentinel (all values) or set of accepted values.
local ANY = {}  -- sentinel: accept any value for this key

local tag_filter = {}
for _, entry in ipairs(config) do
    if not entry.values then
        tag_filter[entry.key] = ANY
    elseif tag_filter[entry.key] ~= ANY then
        if not tag_filter[entry.key] then
            tag_filter[entry.key] = {}
        end
        for _, v in ipairs(entry.values) do
            tag_filter[entry.key][v] = true
        end
    end
    -- If key is already ANY, additional values entries are silently ignored.
end

local function should_import(tags)
    for key, filter in pairs(tag_filter) do
        local v = tags[key]
        if v and (filter == ANY or filter[v]) then
            return true
        end
    end
    return false
end

-- ── Raw tables ────────────────────────────────────────────────────────────────

local tables = {}

tables.node = osm2pgsql.define_node_table('raw_node', {
    { column = 'osm_id',   type = 'int8' },
    { column = 'osm_type', type = 'text' },
    { column = 'tags',     type = 'jsonb' },
    { column = 'geom',     type = 'point', projection = 4326 },
})

tables.line = osm2pgsql.define_way_table('raw_line', {
    { column = 'osm_id',   type = 'int8' },
    { column = 'osm_type', type = 'text' },
    { column = 'tags',     type = 'jsonb' },
    { column = 'geom',     type = 'linestring', projection = 4326 },
})

-- Stores Polygon (closed way) or MultiPolygon (relation).
tables.poly = osm2pgsql.define_area_table('raw_poly', {
    { column = 'osm_id',   type = 'int8' },
    { column = 'osm_type', type = 'text' },
    { column = 'tags',     type = 'jsonb' },
    { column = 'geom',     type = 'geometry', projection = 4326 },
})

-- Stores named route relations (Thames Path, Capital Ring, etc.) as MultiLineString.
tables.route = osm2pgsql.define_relation_table('raw_route', {
    { column = 'osm_id',   type = 'int8' },
    { column = 'osm_type', type = 'text' },
    { column = 'tags',     type = 'jsonb' },
    { column = 'geom',     type = 'multilinestring', projection = 4326 },
})

-- ── Two-stage processing for route relations ──────────────────────────────────
-- Tells osm2pgsql to keep way member geometries available when processing
-- route relations (type=route). Without this, as_multilinestring() returns nil.

function osm2pgsql.select_relation_members(relation)
    if relation.tags.type == 'route' and should_import(relation.tags) then
        local way_ids = {}
        for _, member in ipairs(relation.members) do
            if member.type == 'w' then
                way_ids[#way_ids + 1] = member.ref
            end
        end
        return { ways = way_ids }
    end
end

-- ── Processors ────────────────────────────────────────────────────────────────

function osm2pgsql.process_node(object)
    if not should_import(object.tags) then return end
    tables.node:insert({
        osm_id   = object.id,
        osm_type = 'node',
        tags     = object.tags,
        geom     = object:as_point(),
    })
end

function osm2pgsql.process_way(object)
    if not should_import(object.tags) then return end
    if object.is_closed then
        tables.poly:insert({
            osm_id   = object.id,
            osm_type = 'way',
            tags     = object.tags,
            geom     = object:as_polygon(),
        })
    else
        tables.line:insert({
            osm_id   = object.id,
            osm_type = 'way',
            tags     = object.tags,
            geom     = object:as_linestring(),
        })
    end
end

function osm2pgsql.process_relation(object)
    if object.tags.type == 'multipolygon' or object.tags.type == 'boundary' then
        if not should_import(object.tags) then return end
        local geom = object:as_multipolygon()
        if geom then
            tables.poly:insert({
                osm_id   = object.id,
                osm_type = 'relation',
                tags     = object.tags,
                geom     = geom,
            })
        end
    elseif object.tags.type == 'route' then
        if not should_import(object.tags) then return end
        local geom = object:as_multilinestring()
        if geom then
            tables.route:insert({
                osm_id   = object.id,
                osm_type = 'relation',
                tags     = object.tags,
                geom     = geom,
            })
        end
    end
end
