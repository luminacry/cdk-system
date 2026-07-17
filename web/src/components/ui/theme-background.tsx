"use client"

import { useEffect, useState } from "react"
import { useTheme } from "next-themes"

import { cn } from "@/lib/utils"
import { BackgroundDots } from "@/components/ui/demo"
import { MeshGradient, DotOrbit } from "@paper-design/shaders-react"

interface ThemeBackgroundProps {
  children: React.ReactNode
  className?: string
  shaderVariant?: "mesh" | "dots" | "combined"
}

export function ThemeBackground({
  children,
  className,
  shaderVariant = "mesh",
}: ThemeBackgroundProps) {
  const { resolvedTheme } = useTheme()
  const [mounted, setMounted] = useState(false)

  useEffect(() => {
    setMounted(true)
  }, [])

  // Avoid hydration mismatch: render nothing until mounted, default to dark shader.
  const isDark = mounted ? resolvedTheme === "dark" : true

  return (
    <div
      className={cn(
        "relative min-h-screen overflow-hidden",
        isDark ? "shader-background bg-black" : "light-background",
        className,
      )}
    >
      {isDark ? (
        <DarkShaderBackground variant={shaderVariant} />
      ) : (
        <BackgroundDots />
      )}

      <div className="relative z-10 min-w-0 max-w-full">{children}</div>
    </div>
  )
}

function DarkShaderBackground({ variant }: { variant: "mesh" | "dots" | "combined" }) {
  return (
    <>
      {variant === "mesh" && (
        <MeshGradient
          className="absolute inset-0 size-full"
          colors={["#000000", "#111111", "#2a2a2a", "#444444"]}
          speed={0.4}
          distortion={0.5}
          swirl={0.15}
          grainMixer={0.2}
          grainOverlay={0.05}
        />
      )}

      {variant === "dots" && (
        <DotOrbit
          className="absolute inset-0 size-full"
          colorBack="#000000"
          colors={["#2a2a2a", "#444444"]}
          speed={0.3}
          size={0.25}
          spreading={0.4}
        />
      )}

      {variant === "combined" && (
        <>
          <MeshGradient
            className="absolute inset-0 size-full"
            colors={["#000000", "#111111", "#2a2a2a", "#444444"]}
            speed={0.25}
            distortion={0.5}
            swirl={0.15}
            grainMixer={0.2}
            grainOverlay={0.05}
          />
          <div className="absolute inset-0 opacity-40">
            <DotOrbit
              className="size-full"
              colorBack="transparent"
              colors={["#2a2a2a", "#444444"]}
              speed={0.4}
              size={0.2}
              spreading={0.3}
            />
          </div>
        </>
      )}
    </>
  )
}
