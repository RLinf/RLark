import { useEffect, useRef, type ReactNode } from "react";
import "./StationDetailDialog.css";

// The one open station, in a native modal: the tiles and rows stay exactly where they are while the detail owns
// the top layer. The shell provides the dialog element, the close button and a host slot for the power toast,
// which portals here so the fixed page toast is not hidden behind the modal. Escape and the close event both
// land on onClose; focus lands on the panel title first and returns to the expander through the page.
export function StationDetailDialog({ labelledBy, onClose, children }: {
  labelledBy: string;
  onClose: () => void;
  children: ReactNode;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const dismiss = useRef(onClose);
  dismiss.current = onClose;

  useEffect(() => {
    const element = dialog.current;
    if (!element) return;
    if (!element.open) element.showModal();
    // The panel's title is the dialog's first stop. It is focused here, after showModal, because the panel's own
    // effect runs before the element enters the top layer, when nothing inside can hold focus yet.
    element.querySelector<HTMLElement>("[data-detail-title]")?.focus({ preventScroll: true });
    const closed = () => dismiss.current();
    // A child camera zoom dialog owns Escape while it is open: swallow this dialog's cancel so one key press
    // closes exactly one layer. closedBy=none (set by the page) is the primary guard; this is the fallback.
    const cancel = (event: Event) => {
      if (document.querySelector(".station-camera-dialog[open]")) event.preventDefault();
    };
    element.addEventListener("close", closed);
    element.addEventListener("cancel", cancel);
    // The element goes away with the component; closing it here would fire a close event during StrictMode's
    // effect replay and dismiss the dialog the replay just opened.
    return () => {
      element.removeEventListener("close", closed);
      element.removeEventListener("cancel", cancel);
    };
  }, []);

  return <dialog ref={dialog} className="station-detail-dialog" aria-labelledby={labelledBy}>
    {children}
    <div className="station-toast-host" aria-live="polite" />
  </dialog>;
}
