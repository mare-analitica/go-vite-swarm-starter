// Thin client for the notes API. VITE_API_URL is public (it ends up in the
// bundle); leave it empty in development to use the Vite proxy.
const BASE_URL = (import.meta.env.VITE_API_URL ?? '').replace(/\/$/, '')

export interface Note {
  id: string
  title: string
  body: string
  attachmentName?: string
  createdAt: string
}

interface PresignedUpload {
  url: string
  fields: Record<string, string>
  key: string
}

export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!res.ok) {
    const payload = (await res.json().catch(() => ({}))) as { error?: string }
    throw new ApiError(res.status, payload.error ?? res.statusText)
  }
  return (res.status === 204 ? undefined : await res.json()) as T
}

export const api = {
  listNotes: () => request<Note[]>('/api/notes'),

  createNote: (title: string, body: string) =>
    request<Note>('/api/notes', { method: 'POST', body: JSON.stringify({ title, body }) }),

  deleteNote: (id: string) => request<void>(`/api/notes/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // Uploads go straight to object storage with a presigned POST; the storage
  // enforces the key and the size limit, and the file never passes through the API.
  uploadAttachment: async (id: string, file: File) => {
    const presigned = await request<PresignedUpload>(`/api/notes/${encodeURIComponent(id)}/attachment`, {
      method: 'POST',
      body: JSON.stringify({ filename: file.name }),
    })
    const form = new FormData()
    for (const [key, value] of Object.entries(presigned.fields)) {
      form.append(key, value)
    }
    form.append('file', file)
    const res = await fetch(presigned.url, { method: 'POST', body: form })
    if (!res.ok) {
      const tooLarge = (await res.text()).includes('EntityTooLarge')
      throw new ApiError(res.status, tooLarge ? 'File is too large' : 'Upload failed')
    }
    // The note references the file only after the API verifies it was stored.
    await request(`/api/notes/${encodeURIComponent(id)}/attachment/confirm`, {
      method: 'POST',
      body: JSON.stringify({ key: presigned.key, filename: file.name }),
    })
  },

  attachmentUrl: (id: string) =>
    request<{ url: string }>(`/api/notes/${encodeURIComponent(id)}/attachment`).then((r) => r.url),
}
