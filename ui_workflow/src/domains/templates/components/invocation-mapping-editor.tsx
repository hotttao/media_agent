import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type {
  InvocationExpression,
  InvocationOperation,
  TemplateSlotInput,
} from "@/domains/templates/types";
import { moveInvocationItem } from "@/domains/templates/lib/invocation-editing";

function ExpressionEditor({
  expression,
  path,
  slots,
  onChange,
}: {
  expression: InvocationExpression;
  path: string;
  slots: TemplateSlotInput[];
  onChange: (value: InvocationExpression) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <select
        aria-label={t("mapping.source", { path })}
        value={expression.op}
        className="h-9 w-full rounded-lg border bg-background px-3 text-sm"
        onChange={(event) => {
          const op = event.target.value as InvocationOperation;
          if (op === expression.op) return;
          onChange(
            op === "list"
              ? { op, items: [] }
              : op === "asset_ref"
                ? { op, slot: "" }
                : { op },
          );
        }}
      >
        {["render_template", "asset_ref", "list"].map((op) => (
          <option key={op} value={op}>
            {t(`mapping.${op}`)}
          </option>
        ))}
      </select>
      {expression.op === "asset_ref" ? (
        <select
          aria-label={t("mapping.slot", { path })}
          className="h-9 w-full rounded-lg border bg-background px-3 text-sm"
          value={expression.slot ?? ""}
          onChange={(event) =>
            onChange({ ...expression, slot: event.target.value })
          }
        >
          <option value="">{t("mapping.chooseSlot")}</option>
          {expression.slot &&
          !slots.some(
            (slot) =>
              slot.name === expression.slot && slot.resource_type !== "TEXT",
          ) ? (
            <option value={expression.slot}>
              {t("mapping.brokenSlot", { name: expression.slot })}
            </option>
          ) : null}
          {slots
            .filter((slot) => slot.name && slot.resource_type !== "TEXT")
            .map((slot, index) => (
              <option key={`${slot.name}-${index}`} value={slot.name}>
                {slot.name} · {slot.asset_kind}
              </option>
            ))}
        </select>
      ) : null}
      {expression.op === "render_template" ? (
        <p className="text-xs leading-5 text-muted-foreground">
          {t("mapping.renderHint")}
        </p>
      ) : null}
      {expression.op === "list" ? (
        <div className="space-y-3 border-l-2 border-primary/20 pl-3">
          {(expression.items ?? []).map((item, index) => (
            <div key={index} className="space-y-2 rounded-lg bg-muted/40 p-3">
              <div className="flex items-center justify-between">
                <span className="text-xs text-muted-foreground">
                  {index + 1}
                </span>
                <div className="flex gap-1">
                  <Button
                    size="icon"
                    variant="ghost"
                    type="button"
                    disabled={index === 0}
                    aria-label={t("mapping.up", { path: `${path}[${index}]` })}
                    onClick={() =>
                      onChange(moveInvocationItem(expression, index, -1))
                    }
                  >
                    <ArrowUp className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    type="button"
                    disabled={index === (expression.items?.length ?? 0) - 1}
                    aria-label={t("mapping.down", {
                      path: `${path}[${index}]`,
                    })}
                    onClick={() =>
                      onChange(moveInvocationItem(expression, index, 1))
                    }
                  >
                    <ArrowDown className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    type="button"
                    aria-label={t("mapping.removeItem", {
                      path: `${path}[${index}]`,
                    })}
                    onClick={() =>
                      onChange({
                        ...expression,
                        items: expression.items?.filter(
                          (_, position) => position !== index,
                        ),
                      })
                    }
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
              <ExpressionEditor
                expression={item}
                path={`${path}[${index}]`}
                slots={slots}
                onChange={(next) =>
                  onChange({
                    ...expression,
                    items: expression.items?.map((current, position) =>
                      position === index ? next : current,
                    ),
                  })
                }
              />
            </div>
          ))}
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              onChange({
                ...expression,
                items: [
                  ...(expression.items ?? []),
                  { op: "asset_ref", slot: "" },
                ],
              })
            }
          >
            <Plus className="mr-1 h-3.5 w-3.5" />
            {t("mapping.addItem", { path })}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

export function InvocationMappingEditor({
  value,
  slots,
  onChange,
}: {
  value: Record<string, InvocationExpression>;
  slots: TemplateSlotInput[];
  onChange: (value: Record<string, InvocationExpression>) => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const trimmed = name.trim();
  return (
    <section aria-label={t("mapping.title")} className="space-y-4">
      <div>
        <h3 className="font-semibold">{t("mapping.title")}</h3>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">
          {t("mapping.hint")}
        </p>
      </div>
      {Object.entries(value).map(([parameter, expression]) => (
        <article
          key={parameter}
          aria-label={parameter}
          className="space-y-3 rounded-xl border bg-background p-4"
        >
          <div className="flex items-center justify-between">
            <h4 className="text-sm font-semibold">{parameter}</h4>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t("mapping.removeParameter", { name: parameter })}
              onClick={() =>
                onChange(
                  Object.fromEntries(
                    Object.entries(value).filter(([key]) => key !== parameter),
                  ),
                )
              }
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          </div>
          <ExpressionEditor
            expression={expression}
            path={parameter}
            slots={slots}
            onChange={(next) => onChange({ ...value, [parameter]: next })}
          />
        </article>
      ))}
      <div className="flex gap-2">
        <Input
          aria-label={t("mapping.parameterName")}
          placeholder={t("mapping.parameterName")}
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
        <Button
          type="button"
          variant="outline"
          disabled={!trimmed || Object.hasOwn(value, trimmed)}
          onClick={() => {
            onChange({ ...value, [trimmed]: { op: "render_template" } });
            setName("");
          }}
        >
          {t("mapping.addParameter")}
        </Button>
      </div>
    </section>
  );
}
