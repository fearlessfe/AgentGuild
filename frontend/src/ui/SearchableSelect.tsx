import { type KeyboardEvent, useEffect, useId, useState } from "react";

type SearchableSelectProps<T> = {
  label: string;
  options: T[];
  value: T | null;
  query: string;
  onQueryChange: (query: string) => void;
  onChange: (option: T) => void;
  getOptionKey: (option: T) => string;
  getOptionLabel: (option: T) => string;
  emptyText?: string;
  placeholder?: string;
  disabled?: boolean;
};

export function SearchableSelect<T>({
  label,
  options,
  value,
  query,
  onQueryChange,
  onChange,
  getOptionKey,
  getOptionLabel,
  emptyText = "没有可选项",
  placeholder,
  disabled = false,
}: SearchableSelectProps<T>) {
  const reactID = useId().replace(/:/g, "");
  const inputID = `searchable-select-${reactID}`;
  const listboxID = `${inputID}-listbox`;
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);

  useEffect(() => {
    setActiveIndex((current) => (current >= options.length ? options.length - 1 : current));
  }, [options.length]);

  useEffect(() => {
    if (!disabled) return;
    setOpen(false);
    setActiveIndex(-1);
  }, [disabled]);

  const optionID = (option: T) => `${inputID}-option-${toDOMIDPart(getOptionKey(option))}`;
  const choose = (option: T) => {
    onChange(option);
    onQueryChange(getOptionLabel(option));
    setOpen(false);
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (disabled) return;
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setOpen(true);
      setActiveIndex((current) => (options.length === 0 ? -1 : Math.min(current + 1, options.length - 1)));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setOpen(true);
      setActiveIndex((current) => {
        if (options.length === 0) return -1;
        return current < 0 ? options.length - 1 : Math.max(current - 1, 0);
      });
    } else if (event.key === "Enter" && open && activeIndex >= 0 && options[activeIndex]) {
      event.preventDefault();
      choose(options[activeIndex]);
    } else if (event.key === "Escape") {
      event.preventDefault();
      setOpen(false);
    }
  };

  return (
    <div className="searchable-select">
      <label className="field-label" htmlFor={inputID}>
        {label}
      </label>
      <input
        id={inputID}
        role="combobox"
        aria-autocomplete="list"
        aria-expanded={open}
        aria-controls={open ? listboxID : undefined}
        aria-activedescendant={open && activeIndex >= 0 && options[activeIndex] ? optionID(options[activeIndex]) : undefined}
        autoComplete="off"
        disabled={disabled}
        placeholder={placeholder}
        value={query}
        onChange={(event) => {
          onQueryChange(event.target.value);
          setOpen(true);
          setActiveIndex(-1);
        }}
        onClick={() => {
          if (!disabled) setOpen(true);
        }}
        onFocus={() => {
          if (!disabled) setOpen(true);
        }}
        onBlur={() => setOpen(false)}
        onKeyDown={handleKeyDown}
      />
      {open ? (
        <ul id={listboxID} role="listbox" className="searchable-select__listbox">
          {options.length === 0 ? (
            <li className="searchable-select__empty">{emptyText}</li>
          ) : (
            options.map((option, index) => {
              const key = getOptionKey(option);
              const selected = value != null && getOptionKey(value) === key;
              return (
                <li
                  id={optionID(option)}
                  key={key}
                  role="option"
                  aria-selected={selected}
                  className={index === activeIndex ? "searchable-select__option searchable-select__option--active" : "searchable-select__option"}
                  onMouseEnter={() => setActiveIndex(index)}
                  onMouseDown={(event) => {
                    event.preventDefault();
                    choose(option);
                  }}
                >
                  {getOptionLabel(option)}
                </li>
              );
            })
          )}
        </ul>
      ) : null}
    </div>
  );
}

function toDOMIDPart(value: string): string {
  return Array.from(value, (character) => character.codePointAt(0)?.toString(16) ?? "0").join("-");
}
