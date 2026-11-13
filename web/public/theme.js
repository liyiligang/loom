// Apply the saved palette before the stylesheet and application load. CSP permits this external script.
(() => {
  let preference = 'system'
  try {
    const saved = localStorage.getItem('loom.theme')
    if (saved === 'light' || saved === 'dark') preference = saved
  } catch { /* The system palette is available when storage is blocked. */ }
  const dark = preference === 'dark' || (preference === 'system' && matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
  document.documentElement.classList.toggle('dark', dark)
})()
