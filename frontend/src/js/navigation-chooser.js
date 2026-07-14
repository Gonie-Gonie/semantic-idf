import { t } from "./i18n.js";

let activeChoice = null;

export function chooseSemanticOccurrence(payload = {}) {
  const choices = Array.isArray(payload.occurrences) ? payload.occurrences : [];
  return chooseNavigationOption({
    title: t("semantic.chooseOccurrence", {}, "Choose where to reveal this item"),
    description: payload.selection?.entityId || "",
    choices: choices.map((occurrence) => ({
      value: occurrence.occurrenceId,
      label: occurrence.path || occurrence.occurrenceId,
      detail: occurrence.contextKind || occurrence.preferredView || "",
    })),
  });
}

export function chooseViewTarget(payload = {}) {
  const choices = Array.isArray(payload.targets) ? payload.targets : [];
  return chooseNavigationOption({
    title: t("semantic.chooseViewTarget", {}, "Choose a panel target"),
    description: payload.selection?.entityId || payload.view || "",
    choices: choices.map((target) => ({
      value: target.targetId,
      label: target.label || target.targetId,
      detail: [target.targetKind, target.view].filter(Boolean).join(" / "),
    })),
  });
}

export function closeNavigationChooser() {
  activeChoice?.finish(null);
}

function chooseNavigationOption({ title, description, choices }) {
  if (!choices.length || typeof document === "undefined") {
    return Promise.resolve(null);
  }
  if (choices.length === 1) {
    return Promise.resolve(choices[0].value);
  }
  closeNavigationChooser();
  return new Promise((resolve) => {
    const dialog = document.createElement("dialog");
    dialog.className = "navigation-chooser";
    dialog.setAttribute("aria-labelledby", "navigationChooserTitle");

    const panel = document.createElement("section");
    panel.className = "navigation-chooser__panel";
    const heading = document.createElement("h3");
    heading.id = "navigationChooserTitle";
    heading.textContent = title;
    panel.append(heading);
    if (description) {
      const summary = document.createElement("p");
      summary.className = "navigation-chooser__description";
      summary.textContent = description;
      panel.append(summary);
    }

    const list = document.createElement("div");
    list.className = "navigation-chooser__list";
    list.setAttribute("role", "listbox");
    for (const choice of choices) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "navigation-chooser__option";
      button.setAttribute("role", "option");
      const label = document.createElement("span");
      label.className = "navigation-chooser__label";
      label.textContent = choice.label;
      button.append(label);
      if (choice.detail) {
        const detail = document.createElement("span");
        detail.className = "navigation-chooser__detail";
        detail.textContent = choice.detail;
        button.append(detail);
      }
      button.addEventListener("click", () => activeChoice?.finish(choice.value));
      list.append(button);
    }
    panel.append(list);

    const cancel = document.createElement("button");
    cancel.type = "button";
    cancel.className = "navigation-chooser__cancel";
    cancel.textContent = t("common.cancel", {}, "Cancel");
    cancel.addEventListener("click", () => activeChoice?.finish(null));
    panel.append(cancel);
    dialog.append(panel);

    const finish = (value) => {
      if (!activeChoice || activeChoice.dialog !== dialog) {
        return;
      }
      activeChoice = null;
      dialog.close?.();
      dialog.remove();
      resolve(value);
    };
    activeChoice = { dialog, finish };
    dialog.addEventListener("cancel", (event) => {
      event.preventDefault();
      finish(null);
    });
    dialog.addEventListener("click", (event) => {
      if (event.target === dialog) {
        finish(null);
      }
    });
    document.body.append(dialog);
    if (typeof dialog.showModal === "function") {
      dialog.showModal();
    } else {
      dialog.setAttribute("open", "");
    }
    list.querySelector("button")?.focus();
  });
}
