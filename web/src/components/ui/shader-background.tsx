"use client"

import { MeshGradient, DotOrbit } from "@paper-design/shaders-react"

import { cn } from "@/lib/utils"

interface ShaderBackgroundProps {
  children: React.ReactNode
  className?: string
  variant?: "mesh" | "dots" | "combined"
}

export function ShaderBackground({
  children,
  className,
  variant = "mesh",
}: ShaderBackgroundProps) {
  return (
    <div className={cn("shader-background relative min-h-screen overflow-hidden bg-black", className)}>
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

      <div className="relative z-10">{children}</div>
    </div>
  )
}
