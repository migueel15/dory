-- +goose Up
-- Local: files of a pull request the user marked as reviewed on this node. The
-- hash of the patch at marking time lets the interface drop marks once the
-- file changes.
CREATE TABLE pr_file_reviews (
  repo text NOT NULL,
  number integer NOT NULL,
  path text NOT NULL,
  patch_hash text NOT NULL,
  reviewed_at text NOT NULL,
  PRIMARY KEY (repo, number, path)
);

-- +goose Down
DROP TABLE pr_file_reviews;
