-- +goose Up
-- Files of a pull request the user marked as reviewed. The hash of the file
-- patch at marking time lets the web drop marks once the file changes.
CREATE TABLE IF NOT EXISTS pr_file_reviews (
 repo text NOT NULL,
 number integer NOT NULL,
 path text NOT NULL,
 patch_hash text NOT NULL,
 reviewed_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (repo, number, path)
);

-- +goose Down
DROP TABLE IF EXISTS pr_file_reviews;
