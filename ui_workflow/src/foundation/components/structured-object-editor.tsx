import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Trash2 } from "lucide-react";

function ValueEditor({
  value,
  onChange,
  name,
}: {
  value: unknown;
  onChange: (value: unknown) => void;
  name: string;
}) {
  if (Array.isArray(value))
    return (
      <div className="space-y-2 border-l pl-3">
        {value.map((item, index) => (
          <ValueEditor
            key={index}
            value={item}
            name={`${name} ${index + 1}`}
            onChange={(next) =>
              onChange(
                value.map((current, position) =>
                  position === index ? next : current,
                ),
              )
            }
          />
        ))}
      </div>
    );
  if (value !== null && typeof value === "object")
    return (
      <StructuredObjectEditor
        value={value as Record<string, unknown>}
        onChange={onChange}
      />
    );
  if (typeof value === "boolean")
    return (
      <input
        aria-label={name}
        type="checkbox"
        checked={value}
        onChange={(event) => onChange(event.target.checked)}
      />
    );
  if (typeof value === "number")
    return (
      <Input
        aria-label={name}
        type="number"
        value={value}
        onChange={(event) => {
          const number = Number(event.target.value);
          if (event.target.value && Number.isFinite(number)) onChange(number);
        }}
      />
    );
  return (
    <Textarea
      aria-label={name}
      rows={typeof value === "string" && value.length > 100 ? 3 : 1}
      value={value === null ? "" : String(value ?? "")}
      onChange={(event) => onChange(event.target.value)}
    />
  );
}

/** Edits the existing extension bag without flattening or stringifying its values. */
export function StructuredObjectEditor({
  value,
  onChange,
}: {
  value?: Record<string, unknown>;
  onChange: (value: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  const [key, setKey] = useState("");
  const record = value ?? {};
  return (
    <div className="space-y-3">
      {Object.entries(record).map(([name, content]) => (
        <section key={name} className="space-y-2 rounded-xl bg-muted/30 p-3">
          <div className="flex items-center justify-between">
            <label id={`${id}-${name}`} className="text-xs font-medium">
              {name.replaceAll("_", " ")}
            </label>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t("structuredEditor.remove", { name })}
              onClick={() =>
                onChange(
                  Object.fromEntries(
                    Object.entries(record).filter(([field]) => field !== name),
                  ),
                )
              }
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          </div>
          <ValueEditor
            name={name}
            value={content}
            onChange={(next) => onChange({ ...record, [name]: next })}
          />
        </section>
      ))}
      <div className="flex gap-2">
        <Input
          aria-label={t("structuredEditor.key")}
          placeholder={t("structuredEditor.key")}
          value={key}
          onChange={(event) => setKey(event.target.value)}
        />
        <Button
          type="button"
          variant="outline"
          disabled={!key.trim() || Object.hasOwn(record, key.trim())}
          onClick={() => {
            onChange({ ...record, [key.trim()]: "" });
            setKey("");
          }}
        >
          {t("structuredEditor.add")}
        </Button>
      </div>
    </div>
  );
}
