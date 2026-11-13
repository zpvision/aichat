import { useTranslation } from "react-i18next";
import type { LeadField } from "./leads-api";
export function useFieldLabel() {
  const { t, i18n } = useTranslation();
  return (f: LeadField) =>
    f.labels?.[i18n.resolvedLanguage || "en"] ||
    f.label ||
    t("lead.standard." + f.key, { defaultValue: f.key });
}
export function LeadFields({
  fields,
  values,
  onChange,
}: {
  fields: LeadField[];
  values: Record<string, string | number | null>;
  onChange: (k: string, v: string | number | null) => void;
}) {
  const name = useFieldLabel();
  return (
    <div className="lead-field-inputs">
      {fields
        .filter((f) => f.active)
        .map((f) => (
          <label key={f.key}>
            {name(f)}
            {f.required ? " *" : ""}
            {f.type === "select" ? (
              <select
                value={values[f.key] ?? ""}
                onChange={(e) => onChange(f.key, e.target.value || null)}
              >
                <option value="">—</option>
                {f.options.map((o) => (
                  <option key={o}>{o}</option>
                ))}
              </select>
            ) : (
              <input
                value={values[f.key] ?? ""}
                type={
                  f.type === "number"
                    ? "number"
                    : f.type === "email"
                      ? "email"
                      : f.type === "phone"
                        ? "tel"
                        : "text"
                }
                maxLength={f.type === "text" ? f.max_length || 500 : 2000}
                min={f.min ?? undefined}
                max={f.max ?? undefined}
                step={f.integer ? 1 : "any"}
                onChange={(e) =>
                  onChange(
                    f.key,
                    e.target.value === ""
                      ? null
                      : f.type === "number"
                        ? Number(e.target.value)
                        : e.target.value,
                  )
                }
              />
            )}
          </label>
        ))}
    </div>
  );
}
