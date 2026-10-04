import { useEffect } from 'react'
import { useBlocker } from 'react-router-dom'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from './ui/alert-dialog'

export function UnsavedChanges({ dirty }: { dirty: boolean }) {
  const blocker = useBlocker(({ currentLocation, nextLocation }) =>
    dirty && nextLocation.pathname !== '/login' && currentLocation.pathname !== nextLocation.pathname,
  )
  useEffect(() => {
    if (!dirty) return
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = '' }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [dirty])
  return <AlertDialog open={blocker.state === 'blocked'} onOpenChange={open => { if (!open && blocker.state === 'blocked') blocker.reset() }}>
    <AlertDialogContent>
      <AlertDialogHeader><AlertDialogTitle>离开当前页面？</AlertDialogTitle><AlertDialogDescription>有尚未保存的更改。离开后这些更改将被丢弃。</AlertDialogDescription></AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel onClick={() => { if (blocker.state === 'blocked') blocker.reset() }}>继续编辑</AlertDialogCancel>
        <AlertDialogAction onClick={() => { if (blocker.state === 'blocked') blocker.proceed() }}>放弃更改并离开</AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
}
