package ui

import tea "charm.land/bubbletea/v2"

// copiedMsg reports the extra (tmux) clipboard path.
type copiedMsg struct{ err error }

// copySelected copies the selected item's URL: OSC 52 via Bubble Tea, plus
// the optional copier (tmux load-buffer -w, which forwards OSC 52 from
// inside tmux even when set-clipboard is off).
func (m Model) copySelected() (tea.Model, tea.Cmd) {
	it, ok := m.Selected()
	if !ok || it.URL == "" {
		m.notice = "nothing to copy: no URL"
		return m, nil
	}
	m.notice = "copied " + it.URL
	cmds := []tea.Cmd{tea.SetClipboard(it.URL)}
	if m.copyFn != nil {
		url, fn := it.URL, m.copyFn
		cmds = append(cmds, func() tea.Msg { return copiedMsg{err: fn(url)} })
	}
	return m, tea.Batch(cmds...)
}

func (m Model) onCopied(msg copiedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.withError("copy to tmux buffer", msg.err), nil
	}
	return m, nil
}
