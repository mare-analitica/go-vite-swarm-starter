import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { api, type Note } from './api'
import './App.css'

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : 'Something went wrong'
}

export default function App() {
  const [notes, setNotes] = useState<Note[]>([])
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const refresh = useCallback(async () => {
    try {
      setNotes(await api.listNotes())
      setError('')
    } catch (err) {
      setError(errorMessage(err))
    }
  }, [])

  // Initial load: state is only set from the promise callback, and ignored
  // if the component unmounted in the meantime.
  useEffect(() => {
    let active = true
    api
      .listNotes()
      .then((list) => active && setNotes(list))
      .catch((err: unknown) => active && setError(errorMessage(err)))
    return () => {
      active = false
    }
  }, [])

  async function run(action: () => Promise<unknown>) {
    setBusy(true)
    try {
      await action()
      await refresh()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  function handleCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    void run(async () => {
      await api.createNote(title, body)
      setTitle('')
      setBody('')
    })
  }

  async function handleDownload(id: string) {
    try {
      window.location.assign(await api.attachmentUrl(id))
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  return (
    <main className="container">
      <header>
        <h1>Notes</h1>
        <p className="subtitle">Go API · PostgreSQL · S3 storage · n8n events</p>
      </header>

      <form className="card" onSubmit={handleCreate}>
        <label>
          Title
          <input value={title} onChange={(e) => setTitle(e.target.value)} maxLength={200} required />
        </label>
        <label>
          Body
          <textarea value={body} onChange={(e) => setBody(e.target.value)} maxLength={10000} rows={3} />
        </label>
        <button type="submit" disabled={busy || title.trim() === ''}>
          Add note
        </button>
      </form>

      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}

      <ul className="notes">
        {notes.map((note) => (
          <li key={note.id} className="card">
            <div className="note-header">
              <h2>{note.title}</h2>
              <time dateTime={note.createdAt}>{dateFormat.format(new Date(note.createdAt))}</time>
            </div>
            {note.body && <p>{note.body}</p>}
            <div className="actions">
              {note.attachmentName ? (
                <button type="button" className="link" onClick={() => void handleDownload(note.id)}>
                  Download {note.attachmentName}
                </button>
              ) : (
                <label className="upload">
                  Attach file
                  <input
                    type="file"
                    disabled={busy}
                    onChange={(e) => {
                      const file = e.target.files?.[0]
                      if (file) void run(() => api.uploadAttachment(note.id, file))
                    }}
                  />
                </label>
              )}
              <button
                type="button"
                className="danger"
                disabled={busy}
                onClick={() => void run(() => api.deleteNote(note.id))}
              >
                Delete
              </button>
            </div>
          </li>
        ))}
        {notes.length === 0 && !error && <li className="empty">No notes yet.</li>}
      </ul>
    </main>
  )
}
