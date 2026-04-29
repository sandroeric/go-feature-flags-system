const state = {
  flags: [],
  selectedKey: "",
  mode: "create",
  enabled: false,
  filter: "",
};

const els = {
  storeGeneration: document.getElementById("store-generation"),
  storeVersion: document.getElementById("store-version"),
  serviceStatus: document.getElementById("service-status"),
  flagList: document.getElementById("flag-list"),
  flagListEmpty: document.getElementById("flag-list-empty"),
  editorTitle: document.getElementById("editor-title"),
  flagVersion: document.getElementById("flag-version"),
  banner: document.getElementById("banner"),
  validationErrors: document.getElementById("validation-errors"),
  flagForm: document.getElementById("flag-form"),
  flagKey: document.getElementById("flag-key"),
  flagDefault: document.getElementById("flag-default"),
  enabledToggle: document.getElementById("enabled-toggle"),
  addVariantBtn: document.getElementById("add-variant-btn"),
  variants: document.getElementById("variants"),
  addRuleBtn: document.getElementById("add-rule-btn"),
  rules: document.getElementById("rules"),
  deleteBtn: document.getElementById("delete-btn"),
  newFlagBtn: document.getElementById("new-flag-btn"),
  refreshBtn: document.getElementById("refresh-btn"),
  variantTotal: document.getElementById("variant-total"),
  search: document.getElementById("flag-search"),
  variantTemplate: document.getElementById("variant-template"),
  ruleTemplate: document.getElementById("rule-template"),
};

const emptyFlag = () => ({
  key: "",
  enabled: false,
  default: "control",
  version: 0,
  variants: [
    { name: "control", weight: 50 },
    { name: "treatment", weight: 50 },
  ],
  rules: [
    { attribute: "country", operator: "eq", values: ["BR"], variant: "treatment", priority: 1 },
  ],
});

async function jsonFetch(path, options = {}) {
  const response = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });

  if (response.status === 204) {
    return null;
  }

  const contentType = response.headers.get("content-type") || "";
  const payload = contentType.includes("application/json") ? await response.json() : await response.text();

  if (!response.ok) {
    throw payload;
  }

  return payload;
}

async function refreshHealth() {
  try {
    const health = await jsonFetch("/health");
    els.storeGeneration.textContent = health.store_generation;
    els.storeVersion.textContent = health.store_version;
    els.serviceStatus.textContent = `${health.status} · ${health.flag_count} flags`;
  } catch (error) {
    els.serviceStatus.textContent = "Unavailable";
  }
}

async function loadFlags(preferredKey = state.selectedKey) {
  clearMessages();
  try {
    const flags = await jsonFetch("/flags");
    flags.sort((a, b) => a.key.localeCompare(b.key));
    state.flags = flags;

    const nextKey = flags.some((flag) => flag.key === preferredKey)
      ? preferredKey
      : flags[0] ? flags[0].key : "";

    if (nextKey) {
      selectFlag(nextKey);
    } else {
      startNewFlag();
    }

    renderFlagList();
    refreshHealth();
  } catch (error) {
    showBanner("Could not load flags. Check the API server and try again.", true);
  }
}

function renderFlagList() {
  const filter = state.filter.trim().toLowerCase();
  const flags = state.flags.filter((flag) => flag.key.toLowerCase().includes(filter));

  els.flagList.innerHTML = "";
  els.flagListEmpty.classList.toggle("hidden", flags.length > 0);

  flags.forEach((flag) => {
    const item = document.createElement("button");
    item.type = "button";
    item.className = `flag-item${flag.key === state.selectedKey ? " active" : ""}`;
    item.innerHTML = `
      <div class="flag-item-top">
        <span class="flag-key">${escapeHTML(flag.key)}</span>
        <span class="pill ${flag.enabled ? "" : "disabled"}">${flag.enabled ? "Enabled" : "Disabled"}</span>
      </div>
      <div class="flag-meta">
        <span class="pill">v${flag.version}</span>
        <span class="pill">${flag.variants.length} variants</span>
        <span class="pill">${flag.rules.length} rules</span>
      </div>
      <p class="flag-subtext">Default: ${escapeHTML(flag.default)}</p>
    `;
    item.addEventListener("click", () => {
      selectFlag(flag.key);
      renderFlagList();
    });
    els.flagList.appendChild(item);
  });
}

function selectFlag(key) {
  const flag = state.flags.find((item) => item.key === key);
  if (!flag) {
    return;
  }

  state.selectedKey = key;
  state.mode = "edit";
  populateForm(flag);
}

function startNewFlag() {
  state.selectedKey = "";
  state.mode = "create";
  populateForm(emptyFlag());
  renderFlagList();
}

function populateForm(flag) {
  els.editorTitle.textContent = state.mode === "edit" ? `Editing ${flag.key}` : "Create a Flag";
  els.flagKey.value = flag.key || "";
  els.flagKey.disabled = state.mode === "edit";
  els.flagDefault.value = flag.default || "";
  els.flagVersion.textContent = String(flag.version || 0);
  state.enabled = !!flag.enabled;
  syncEnabledToggle();
  els.deleteBtn.disabled = state.mode !== "edit";
  renderVariants(flag.variants || []);
  renderRules(flag.rules || []);
  updateVariantTotal();
}

function renderVariants(variants) {
  els.variants.innerHTML = "";
  variants.forEach((variant) => addVariantRow(variant));
}

function renderRules(rules) {
  els.rules.innerHTML = "";
  rules.forEach((rule) => addRuleRow(rule));
}

function addVariantRow(variant = { name: "", weight: 0 }) {
  const fragment = els.variantTemplate.content.cloneNode(true);
  const row = fragment.querySelector(".variant-row");
  row.querySelector('[data-field="name"]').value = variant.name || "";
  row.querySelector('[data-field="weight"]').value = variant.weight ?? 0;
  row.querySelector('[data-action="remove"]').addEventListener("click", () => {
    row.remove();
    updateVariantTotal();
  });
  row.querySelector('[data-field="weight"]').addEventListener("input", updateVariantTotal);
  els.variants.appendChild(fragment);
  updateVariantTotal();
}

function addRuleRow(rule = { attribute: "", operator: "eq", values: [], variant: "", priority: 0 }) {
  const fragment = els.ruleTemplate.content.cloneNode(true);
  const row = fragment.querySelector(".rule-row");
  row.querySelector('[data-field="attribute"]').value = rule.attribute || "";
  row.querySelector('[data-field="operator"]').value = rule.operator || "eq";
  row.querySelector('[data-field="values"]').value = Array.isArray(rule.values) ? rule.values.join(", ") : "";
  row.querySelector('[data-field="variant"]').value = rule.variant || "";
  row.querySelector('[data-field="priority"]').value = rule.priority ?? 0;
  row.querySelector('[data-action="remove"]').addEventListener("click", () => row.remove());
  els.rules.appendChild(fragment);
}

function updateVariantTotal() {
  const total = Array.from(els.variants.querySelectorAll(".variant-row"))
    .map((row) => Number(row.querySelector('[data-field="weight"]').value || 0))
    .reduce((sum, value) => sum + value, 0);
  els.variantTotal.textContent = `Total weight: ${total}`;
}

function readForm() {
  const variants = Array.from(els.variants.querySelectorAll(".variant-row")).map((row) => ({
    name: row.querySelector('[data-field="name"]').value.trim(),
    weight: Number(row.querySelector('[data-field="weight"]').value || 0),
  }));

  const rules = Array.from(els.rules.querySelectorAll(".rule-row")).map((row) => ({
    attribute: row.querySelector('[data-field="attribute"]').value.trim(),
    operator: row.querySelector('[data-field="operator"]').value,
    values: row.querySelector('[data-field="values"]').value.split(",").map((value) => value.trim()).filter(Boolean),
    variant: row.querySelector('[data-field="variant"]').value.trim(),
    priority: Number(row.querySelector('[data-field="priority"]').value || 0),
  }));

  return {
    key: els.flagKey.value.trim(),
    enabled: state.enabled,
    default: els.flagDefault.value.trim(),
    variants,
    rules,
  };
}

async function saveFlag(event) {
  event.preventDefault();
  clearMessages();

  const payload = readForm();
  const isEdit = state.mode === "edit";
  const path = isEdit ? `/flags/${encodeURIComponent(state.selectedKey)}` : "/flags";
  const method = isEdit ? "PUT" : "POST";

  try {
    const saved = await jsonFetch(path, {
      method,
      body: JSON.stringify(payload),
    });
    showBanner(`Saved ${saved.key} successfully.`);
    await loadFlags(saved.key);
  } catch (error) {
    showAPIError(error, "Could not save the flag.");
  }
}

async function deleteFlag() {
  if (state.mode !== "edit" || !state.selectedKey) {
    return;
  }
  clearMessages();

  const confirmed = window.confirm(`Delete flag "${state.selectedKey}"?`);
  if (!confirmed) {
    return;
  }

  try {
    await jsonFetch(`/flags/${encodeURIComponent(state.selectedKey)}`, { method: "DELETE" });
    showBanner(`Deleted ${state.selectedKey}.`);
    await loadFlags("");
  } catch (error) {
    showAPIError(error, "Could not delete the flag.");
  }
}

function showAPIError(error, fallbackMessage) {
  const apiError = error && typeof error === "object" ? error.error : null;
  showBanner(apiError && apiError.message ? apiError.message : fallbackMessage, true);

  if (!apiError || !Array.isArray(apiError.details) || apiError.details.length === 0) {
    els.validationErrors.classList.add("hidden");
    els.validationErrors.innerHTML = "";
    return;
  }

  els.validationErrors.innerHTML = `<ul>${apiError.details
    .map((detail) => `<li><strong>${escapeHTML(detail.field)}:</strong> ${escapeHTML(detail.message)}</li>`)
    .join("")}</ul>`;
  els.validationErrors.classList.remove("hidden");
}

function showBanner(message, isError = false) {
  els.banner.textContent = message;
  els.banner.classList.remove("hidden");
  els.banner.classList.toggle("validation-box", false);
  els.banner.style.background = isError ? "rgba(166, 50, 50, 0.12)" : "";
  els.banner.style.color = isError ? "var(--danger)" : "";
}

function clearMessages() {
  els.banner.classList.add("hidden");
  els.banner.textContent = "";
  els.banner.style.background = "";
  els.banner.style.color = "";
  els.validationErrors.classList.add("hidden");
  els.validationErrors.innerHTML = "";
}

function syncEnabledToggle() {
  els.enabledToggle.classList.toggle("active", state.enabled);
  els.enabledToggle.setAttribute("aria-pressed", String(state.enabled));
  els.enabledToggle.textContent = state.enabled ? "Enabled" : "Disabled";
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

els.enabledToggle.addEventListener("click", () => {
  state.enabled = !state.enabled;
  syncEnabledToggle();
});

els.addVariantBtn.addEventListener("click", () => addVariantRow());
els.addRuleBtn.addEventListener("click", () => addRuleRow());
els.newFlagBtn.addEventListener("click", () => {
  clearMessages();
  startNewFlag();
});
els.refreshBtn.addEventListener("click", () => loadFlags());
els.deleteBtn.addEventListener("click", deleteFlag);
els.flagForm.addEventListener("submit", saveFlag);
els.search.addEventListener("input", (event) => {
  state.filter = event.target.value;
  renderFlagList();
});

loadFlags();
refreshHealth();
