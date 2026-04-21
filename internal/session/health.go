package session

import "github.com/routerarchitects/nats-agent-core/agentcore"

func (m *Manager) setStateLocked(state agentcore.ConnectionState) {
	m.health.State = state
	if m.hooks.Metrics != nil {
		m.hooks.Metrics.SetConnectionState(string(state))
	}
}

func (m *Manager) setConnectedLocked(connectedURL string, jsReady, kvReady bool) {
	m.health.ConnectedURL = connectedURL
	m.health.JetStreamReady = jsReady
	m.health.KVReady = kvReady
	m.health.LastError = ""
	if jsReady && kvReady {
		m.setStateLocked(agentcore.StateConnected)
		return
	}
	m.setStateLocked(agentcore.StateDegraded)
}

func (m *Manager) setReconnectingLocked(lastError error) {
	m.health.JetStreamReady = false
	m.health.KVReady = false
	if lastError != nil {
		m.health.LastError = lastError.Error()
	}
	m.setStateLocked(agentcore.StateReconnecting)
}

func (m *Manager) setDegradedLocked(lastError error) {
	m.health.JetStreamReady = false
	m.health.KVReady = false
	if lastError != nil {
		m.health.LastError = lastError.Error()
	}
	m.setStateLocked(agentcore.StateDegraded)
}

func (m *Manager) setClosedLocked(lastError error) {
	m.health.ConnectedURL = ""
	m.health.JetStreamReady = false
	m.health.KVReady = false
	if lastError != nil {
		m.health.LastError = lastError.Error()
	}
	m.setStateLocked(agentcore.StateClosed)
}
