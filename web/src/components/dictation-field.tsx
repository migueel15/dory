import { useId, useRef, type ComponentProps } from "react";
import { DictationMic, DictationPreview, useLocalDictation, useEnterFinishesDictation } from "./local-dictation";
import { ErrorText, Textarea } from "./ui";

type Props = Omit<ComponentProps<typeof Textarea>, "value" | "onChange" | "ref"> & {
  value: string;
  onValue: (text: string) => void;
  label?: string;
};

export function DictationField({ value, onValue, label, id, className, disabled, readOnly, onKeyDown, ...rest }: Props) {
  const fallbackId = useId();
  const inputId = id ?? fallbackId;
  const input = useRef<HTMLTextAreaElement>(null);
  const dictation = useLocalDictation(value, onValue, input);
  useEnterFinishesDictation(dictation, input);
  return (
    <div data-dictation-panel className="min-w-0 space-y-1">
      {label && <label htmlFor={inputId} className="block text-sm text-slate-600 dark:text-text-secondary">{label}</label>}
      {dictation.preview && <DictationPreview value={dictation.value} range={dictation.preview}
        className={`rounded-md border border-slate-300 bg-white !px-2 !py-1.5 !text-sm dark:border-border-strong dark:bg-surface ${className ?? ""}`} />}
      <Textarea {...rest} id={inputId} ref={input} value={dictation.value} disabled={disabled}
        readOnly={readOnly || dictation.active} onChange={(e) => onValue(e.currentTarget.value)}
        onKeyDown={(e) => { if (!dictation.active || e.key === "Escape") onKeyDown?.(e); }}
        className={dictation.preview ? "sr-only" : className} />
      <div className="flex items-start justify-end gap-2">
        <ErrorText error={dictation.error} />
        <DictationMic dictation={dictation} disabled={disabled || readOnly} />
      </div>
    </div>
  );
}
