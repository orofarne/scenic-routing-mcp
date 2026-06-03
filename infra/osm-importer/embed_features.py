#!/usr/bin/env python3
"""
Compute 256-dim nomic-embed-text vectors for features where poi_vec IS NULL.

For each feature: OSM tags → dictionary lookup → text profile → embedding → poi_vec.
Reads tag→description pairs from /scripts/osm_tags.csv (key,value,description).
Only processes rows with poi_vec IS NULL so it is safe to re-run.

Key optimisation: many features share identical tag combinations and therefore
produce identical text profiles. Profiles are deduplicated before embedding so
that each unique profile is embedded exactly once regardless of how many features
share it.

Environment:
    POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB  — DB credentials (host: db)
    OSM_TAGS_CSV                                   — path to dictionary CSV
                                                     (default: /scripts/osm_tags.csv)
    OLLAMA_BASE_URL                                — Ollama API base URL
                                                     (default: http://ollama:11434)
"""

import csv
import json
import os
from collections import defaultdict

import numpy as np
import psycopg2
import requests
from psycopg2.extras import execute_values

DICT_PATH    = os.getenv('OSM_TAGS_CSV', '/scripts/osm_tags.csv')
OLLAMA_URL   = os.getenv('OLLAMA_BASE_URL', 'http://ollama:11434')
MODEL_NAME   = 'nomic-embed-text'
EMBED_DIM    = 256
EMBED_BATCH  = 32    # texts per Ollama /api/embed call
UPDATE_CHUNK = 2000  # rows per UPDATE statement


def log(msg: str) -> None:
    print(f'[embed] {msg}', flush=True)


def load_dictionary(path: str) -> dict:
    """Returns {(key, value): description} for rows with non-empty descriptions."""
    d = {}
    with open(path, newline='', encoding='utf-8') as f:
        reader = csv.DictReader(line for line in f if not line.startswith('#'))
        for row in reader:
            if row['description']:
                d[(row['key'], row['value'])] = row['description']
    return d


def build_profile(tags: dict, dictionary: dict) -> str:
    """Concatenate descriptions for tags found in the dictionary, then append name."""
    parts = [dictionary[k, v] for k, v in tags.items() if (k, v) in dictionary]
    for key in ('name', 'description'):
        if tags.get(key):
            parts.append(tags[key])
    return '. '.join(parts)


def embed_batch(texts: list[str]) -> list[list[float]]:
    """Embed a batch of texts via Ollama, truncate to EMBED_DIM, re-normalize."""
    resp = requests.post(
        f'{OLLAMA_URL}/api/embed',
        json={'model': MODEL_NAME, 'input': texts},
        timeout=120,
    )
    resp.raise_for_status()
    vecs = np.array(resp.json()['embeddings'], dtype=np.float32)
    vecs = vecs[:, :EMBED_DIM]  # Matryoshka truncation
    norms = np.linalg.norm(vecs, axis=1, keepdims=True)
    vecs = vecs / np.where(norms > 0, norms, 1.0)
    return vecs.tolist()


def db_url() -> str:
    u = os.getenv('POSTGRES_USER', 'scenic')
    p = os.getenv('POSTGRES_PASSWORD', 'scenic')
    d = os.getenv('POSTGRES_DB', 'scenic')
    return f'postgresql://{u}:{p}@db:5432/{d}'


def main() -> None:
    log('Loading dictionary...')
    dictionary = load_dictionary(DICT_PATH)
    log(f'{len(dictionary)} tag descriptions loaded')

    conn = psycopg2.connect(db_url())

    # Load all pending features in one query.
    log('Fetching pending features...')
    with conn.cursor() as cur:
        cur.execute('SELECT id, tags FROM feature WHERE poi_vec IS NULL')
        rows = cur.fetchall()

    total = len(rows)
    log(f'{total} features need embedding')
    if total == 0:
        log('Nothing to do')
        conn.close()
        return

    # Group feature IDs by text profile — identical profiles share one embed call.
    profile_to_ids: dict[str, list[int]] = defaultdict(list)
    for fid, tags in rows:
        profile_to_ids[build_profile(tags, dictionary)].append(fid)

    unique_profiles = list(profile_to_ids.keys())
    saved = total - len(unique_profiles)
    log(f'{len(unique_profiles)} unique profiles ({saved} duplicate embed calls saved)')

    # Embed unique profiles in batches via Ollama.
    profile_to_vec: dict[str, list[float]] = {}
    for i in range(0, len(unique_profiles), EMBED_BATCH):
        batch = unique_profiles[i : i + EMBED_BATCH]
        texts = ['search_document: ' + (p or 'feature') for p in batch]
        vecs = embed_batch(texts)
        for profile, vec in zip(batch, vecs):
            profile_to_vec[profile] = vec
        log(f'embedded {min(i + EMBED_BATCH, len(unique_profiles))}/{len(unique_profiles)} unique profiles')

    # Expand vectors back to all feature IDs.
    pairs = [
        (fid, json.dumps(profile_to_vec[profile]))
        for profile, ids in profile_to_ids.items()
        for fid in ids
    ]

    # Write to DB in chunks.
    log(f'Updating {len(pairs)} rows...')
    for i in range(0, len(pairs), UPDATE_CHUNK):
        chunk = pairs[i : i + UPDATE_CHUNK]
        with conn.cursor() as cur:
            execute_values(
                cur,
                '''UPDATE feature AS f
                   SET poi_vec = d.v::vector
                   FROM (VALUES %s) AS d(id, v)
                   WHERE f.id = d.id::bigint''',
                chunk,
            )
        conn.commit()
        log(f'updated {min(i + UPDATE_CHUNK, len(pairs))}/{len(pairs)}')

    conn.close()
    log('Complete')


if __name__ == '__main__':
    main()
