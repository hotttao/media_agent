import { CircleAlert, LoaderCircle } from "lucide-react";
import type { FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { OryBrowserFlow, OryUiNode } from "@/foundation/lib/ory-flow";
import { cn } from "@/foundation/lib/utils";
import { PasswordField } from "@/foundation/components/password-field";

interface OryFlowFormProps {
  kind: "login" | "registration";
  flow: OryBrowserFlow;
  isSubmitting: boolean;
  error: string | null;
  onSubmit: (values: Record<string, string>) => Promise<void>;
}

function fieldID(node: OryUiNode): string {
  return `ory-${node.attributes.name.replaceAll(/[^a-zA-Z0-9_-]/g, "-")}`;
}

export function OryFlowForm({
  kind,
  flow,
  isSubmitting,
  error,
  onSubmit,
}: OryFlowFormProps) {
  const { t } = useTranslation();
  const inputNodes = flow.ui.nodes.filter(
    (node) =>
      node.type === "input" &&
      !["hidden", "submit"].includes(node.attributes.type),
  );
  const hiddenNodes = flow.ui.nodes.filter(
    (node) => node.type === "input" && node.attributes.type === "hidden",
  );
  const submitNodes = flow.ui.nodes.filter(
    (node) => node.type === "input" && node.attributes.type === "submit",
  );
  const messages = flow.ui.messages ?? [];

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const nativeEvent = event.nativeEvent as SubmitEvent;
    const data = new FormData(event.currentTarget);
    const submitter = nativeEvent.submitter;
    if (submitter instanceof HTMLButtonElement && submitter.name) {
      data.set(submitter.name, submitter.value);
    }
    const values: Record<string, string> = {};
    for (const [name, value] of data.entries()) {
      if (typeof value === "string") values[name] = value;
    }
    await onSubmit(values);
  };

  return (
    <form className="space-y-5" onSubmit={handleSubmit}>
      {hiddenNodes.map((node) => (
        <input
          key={`${node.group}-${node.attributes.name}`}
          name={node.attributes.name}
          type="hidden"
          value={node.attributes.value ?? ""}
        />
      ))}

      {(error || messages.length > 0) && (
        <Alert variant="destructive" className="bg-destructive/5">
          <CircleAlert className="h-4 w-4" />
          <AlertDescription>
            {error ? (
              <p>{error}</p>
            ) : (
              messages.map((message) => <p key={message.id}>{message.text}</p>)
            )}
          </AlertDescription>
        </Alert>
      )}

      {inputNodes.map((node) => {
        const attributes = node.attributes;
        const id = fieldID(node);
        const label = t(`auth.fields.${attributes.name.replaceAll(".", "_")}`, {
          defaultValue: node.meta.label?.text ?? attributes.name,
        });
        const Field = attributes.type === "password" ? PasswordField : Input;
        return (
          <div className="space-y-2" key={`${node.group}-${attributes.name}`}>
            <Label htmlFor={id}>
              {label}
              {attributes.required ? " *" : ""}
            </Label>
            <Field
              id={id}
              name={attributes.name}
              {...(attributes.type === "password"
                ? {}
                : { type: attributes.type })}
              defaultValue={attributes.value ?? ""}
              required={attributes.required}
              disabled={attributes.disabled || isSubmitting}
              autoComplete={attributes.autocomplete}
              className={cn(
                "h-11 bg-background/70 px-3.5 shadow-none transition-[border-color,box-shadow]",
                node.messages.length > 0 &&
                  "border-destructive focus-visible:ring-destructive",
              )}
              aria-describedby={
                node.messages.length > 0 ? `${id}-message` : undefined
              }
            />
            {node.messages.length > 0 && (
              <div
                id={`${id}-message`}
                className="space-y-1 text-xs text-destructive"
              >
                {node.messages.map((message) => (
                  <p key={message.id}>{message.text}</p>
                ))}
              </div>
            )}
          </div>
        );
      })}

      <div className="space-y-3 pt-1">
        {submitNodes.map((node) => (
          <Button
            className="h-11 w-full"
            disabled={node.attributes.disabled || isSubmitting}
            key={`${node.group}-${node.attributes.value}`}
            name={node.attributes.name}
            type="submit"
            value={node.attributes.value}
          >
            {isSubmitting && <LoaderCircle className="h-4 w-4 animate-spin" />}
            {node.attributes.value === "password"
              ? t(kind === "registration" ? "auth.register" : "auth.signIn")
              : t(`auth.submit.${node.attributes.value ?? "continue"}`, {
                  defaultValue: node.meta.label?.text ?? t("auth.continue"),
                })}
          </Button>
        ))}
      </div>
    </form>
  );
}
