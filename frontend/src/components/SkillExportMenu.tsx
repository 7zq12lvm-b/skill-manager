import { useLayoutEffect, useRef } from "react";
import { createPortal } from "react-dom";
import { Download } from "lucide-react";

type Props = {
  x: number;
  y: number;
  unavailable: boolean;
  onClose: () => void;
  onExport: (format: "zip" | "skill") => void;
};

export function SkillExportMenu({ x, y, unavailable, onClose, onExport }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const menu = ref.current!;
    const rect = menu.getBoundingClientRect();
    menu.style.left = `${Math.max(8, Math.min(x, window.innerWidth - rect.width - 8))}px`;
    menu.style.top = `${Math.max(8, Math.min(y, window.innerHeight - rect.height - 8))}px`;
    (menu.querySelector<HTMLButtonElement>("button:not(:disabled)") || menu).focus();
    const outside = (event: PointerEvent) => {
      if (!menu.contains(event.target as Node)) onClose();
    };
    const close = () => onClose();
    document.addEventListener("pointerdown", outside);
    window.addEventListener("resize", close);
    window.addEventListener("blur", close);
    window.addEventListener("scroll", close, true);
    return () => {
      document.removeEventListener("pointerdown", outside);
      window.removeEventListener("resize", close);
      window.removeEventListener("blur", close);
      window.removeEventListener("scroll", close, true);
    };
  }, [x, y, onClose]);

  return createPortal(
    <div
      ref={ref}
      role="menu"
      aria-label="导出 Skill"
      tabIndex={-1}
      className="fixed z-50 w-52 max-w-[calc(100vw-1rem)] rounded-md border border-border bg-white p-1 text-sm shadow-lg"
      style={{ left: x, top: y }}
      onContextMenu={(event) => event.preventDefault()}
      onKeyDown={(event) => {
        if (event.key === "Escape" || event.key === "Tab") {
          event.preventDefault();
          onClose();
        }
        if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
          event.preventDefault();
          const buttons = Array.from(ref.current!.querySelectorAll<HTMLButtonElement>("button:not(:disabled)"));
          if (!buttons.length) return;
          const current = buttons.indexOf(document.activeElement as HTMLButtonElement);
          const next = event.key === "Home" ? 0 : event.key === "End" ? buttons.length - 1 : (current + (event.key === "ArrowUp" ? -1 : 1) + buttons.length) % buttons.length;
          buttons[next].focus();
        }
      }}
    >
      {(["zip", "skill"] as const).map((format) => (
        <button
          key={format}
          role="menuitem"
          type="button"
          disabled={unavailable}
          className="flex w-full items-center gap-2 rounded px-3 py-2 text-left hover:bg-blue-50 focus:bg-blue-50 focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
          onClick={() => onExport(format)}
        >
          <Download aria-hidden="true" className="h-4 w-4" />
          导出为 .{format}
        </button>
      ))}
      {unavailable && <p className="px-3 py-2 text-xs text-muted-foreground">本地 Skill 目录不可用</p>}
    </div>,
    document.body,
  );
}
