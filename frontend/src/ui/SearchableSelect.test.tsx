import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { SearchableSelect } from "./SearchableSelect";

type Option = { id: string; name: string };

const options: Option[] = [
  { id: "api", name: "acme/api" },
  { id: "web", name: "acme/web" },
];

function ControlledSelect({ disabled = false, emptyText = "没有匹配的仓库", items = options }: { disabled?: boolean; emptyText?: string; items?: Option[] }) {
  const [query, setQuery] = useState("");
  const [value, setValue] = useState<Option | null>(null);
  return (
    <SearchableSelect
      label="授权仓库"
      options={items}
      value={value}
      query={query}
      onQueryChange={setQuery}
      onChange={setValue}
      getOptionKey={(option) => option.id}
      getOptionLabel={(option) => option.name}
      emptyText={emptyText}
      disabled={disabled}
    />
  );
}

describe("SearchableSelect", () => {
  it("exposes an editable combobox and only links the listbox while open", async () => {
    const user = userEvent.setup();
    render(<ControlledSelect />);

    const input = screen.getByRole("combobox", { name: "授权仓库" });
    expect(input).toHaveAttribute("aria-expanded", "false");
    expect(input).not.toHaveAttribute("aria-controls");

    await user.click(input);

    expect(input).toHaveAttribute("aria-expanded", "true");
    expect(input).toHaveAttribute("aria-controls", screen.getByRole("listbox").id);
    expect(screen.getAllByRole("option")).toHaveLength(2);
  });

  it("moves the active option with arrows and selects it with Enter", async () => {
    const user = userEvent.setup();
    render(<ControlledSelect />);
    const input = screen.getByRole("combobox", { name: "授权仓库" });

    await user.click(input);
    await user.keyboard("{ArrowDown}{ArrowDown}");
    const web = screen.getByRole("option", { name: "acme/web" });
    expect(input).toHaveAttribute("aria-activedescendant", web.id);
    await user.keyboard("{Enter}");

    expect(input).toHaveValue("acme/web");
    expect(input).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("supports ArrowUp and Escape without selecting", async () => {
    const user = userEvent.setup();
    render(<ControlledSelect />);
    const input = screen.getByRole("combobox", { name: "授权仓库" });

    await user.click(input);
    await user.keyboard("{ArrowUp}");
    expect(input).toHaveAttribute("aria-activedescendant", screen.getByRole("option", { name: "acme/web" }).id);
    await user.keyboard("{Escape}");

    expect(input).toHaveValue("");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("keeps focus on the input while selecting with the mouse", () => {
    const onChange = vi.fn();
    render(
      <SearchableSelect
        label="授权仓库"
        options={options}
        value={null}
        query=""
        onQueryChange={() => undefined}
        onChange={onChange}
        getOptionKey={(option) => option.id}
        getOptionLabel={(option) => option.name}
      />,
    );
    const input = screen.getByRole("combobox", { name: "授权仓库" });
    input.focus();
    fireEvent.click(input);

    fireEvent.mouseDown(screen.getByRole("option", { name: "acme/api" }));

    expect(onChange).toHaveBeenCalledWith(options[0]);
    expect(input).toHaveFocus();
  });

  it("renders empty text outside the option set", async () => {
    const user = userEvent.setup();
    render(<ControlledSelect items={[]} />);

    await user.click(screen.getByRole("combobox", { name: "授权仓库" }));

    expect(screen.getByText("没有匹配的仓库")).toBeVisible();
    expect(screen.queryByRole("option")).not.toBeInTheDocument();
  });

  it("does not open or accept keyboard input while disabled", async () => {
    const user = userEvent.setup();
    render(<ControlledSelect disabled />);
    const input = screen.getByRole("combobox", { name: "授权仓库" });

    expect(input).toBeDisabled();
    await user.click(input);
    await user.keyboard("{ArrowDown}");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });
});
