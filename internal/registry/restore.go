package registry

// RestoreRecords returns activation records for reconnect restoration.
func (r *Registry) RestoreRecords() []ActivationRecord {
	return r.ListActivations()
}
