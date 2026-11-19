# Feedback and states

## 009 Overlays (rule)

- Applies: Any product with tooltips, popovers, confirmations, drawers or dialogs.
- Signals: Tooltip, Popover, Popconfirm, Dialog, Modal, Drawer, Sheet, confirm(; a dialog for a one-item delete; a drawer used to confirm; a tooltip holding buttons; two components doing the same job.
- Skip: Products with almost no floating UI.
- Demo: https://vibedesign.tips/009-overlays/
- Text: tips/009-overlays.md (5.2k characters)

## 016 Toast or notification (rule)

- Applies: Any product that tells people something happened.
- Signals: toast(, sonner, react-hot-toast, notistack, message.success, notification.open, Snackbar, alert(; a toast for an error on a form field; a toast for something another person did; no undo on a destructive action.
- Skip: None: nearly every product has messages.
- Demo: https://vibedesign.tips/016-toast-or-notification/
- Text: tips/016-toast-or-notification.md (11.1k characters)

## 017 Loading states (rule)

- Applies: Any wait: page loads, lists, buttons that call the server, uploads, AI answers.
- Signals: isLoading or loading state; Spinner, Skeleton, Suspense fallback; useQuery, SWR, Apollo; a spinner centred on an empty page; a blocking overlay while saving; layout that jumps when data arrives.
- Skip: Static pages with no data loading.
- Demo: https://vibedesign.tips/017-loading-states/
- Text: tips/017-loading-states.md (12.2k characters)
