package storage

type CleanupResult struct {
	DeletedIDs []int64 `json:"deletedIds"`
}

func (s *Store) Cleanup(policy CleanupPolicy) error {
	_, err := s.CleanupWithResult(policy)
	return err
}

// CleanupWithResult reports committed deletions, including successful earlier
// batches if a later policy fails, so a live list can remove the same records.
func (s *Store) CleanupWithResult(policy CleanupPolicy) (CleanupResult, error) {
	result := CleanupResult{DeletedIDs: []int64{}}
	defer func() { _ = s.RetryImageDeletes() }()
	deleteBatch := func(ids []int64, queryErr error) error {
		if queryErr != nil {
			return queryErr
		}
		deleted, err := s.deleteIDsWithResult(ids)
		result.DeletedIDs = append(result.DeletedIDs, deleted...)
		return err
	}
	if policy.MaxDays > 0 {
		cutoff := int64(policy.MaxDays) * 24 * 60 * 60 * 1000
		if err := deleteBatch(s.idsWhere(`pinned = 0 AND timestamp < (strftime('%s','now') * 1000 - ?)`, cutoff)); err != nil {
			return result, err
		}
	}
	if policy.MaxClips > 0 {
		if err := deleteBatch(s.idsBeyondLimit(policy.MaxClips)); err != nil {
			return result, err
		}
	}
	if policy.MaxImageMB > 0 {
		if err := deleteBatch(s.imageIDsOverLimit(int64(policy.MaxImageMB) * 1024 * 1024)); err != nil {
			return result, err
		}
	}
	return result, nil
}
