import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { AppShell } from "./app/AppShell";
import "./styles/tokens.css";

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: 1, staleTime: 1000 } } });
createRoot(document.getElementById("root")!).render(<StrictMode><QueryClientProvider client={queryClient}><BrowserRouter><AppShell /></BrowserRouter></QueryClientProvider></StrictMode>);
